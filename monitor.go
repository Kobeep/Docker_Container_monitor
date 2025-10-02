package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/fatih/color"
	"github.com/kevinburke/ssh_config"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type ServiceCheckResult struct {
	Container string `json:"container"`
	Port      string `json:"port"`
	Status    string `json:"status"`
}

// ContainerStat represents resource usage statistics for a container.
type ContainerStat struct {
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkRx     uint64  `json:"network_rx"`
	NetworkTx     uint64  `json:"network_tx"`
}

func main() {
	app := &cli.App{
		Name:    "monitor",
		Usage:   "Monitor Docker containers, services and events (local and remote)",
		Version: "1.1.0",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output in JSON format",
			},
			&cli.StringFlag{
				Name:    "filter",
				Aliases: []string{"f"},
				Usage:   "Filter containers (e.g., 'name=nginx' or 'status=running')",
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "state",
				Usage:  "Show container names and states",
				Action: stateOnly,
			},
			{
				Name:   "service",
				Usage:  "Show service statuses",
				Action: serviceOnly,
			},
			{
				Name:  "remote",
				Usage: "Monitor remote Docker via SSH",
				Description: `Connect to a remote Docker host via SSH.
Options:
- Use SSH config: monitor remote --host <alias>
- Use manual: monitor remote <user>@<host> -i <sshkey>`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "host",
						Usage: "SSH config alias",
					},
					&cli.StringFlag{
						Name:  "i",
						Usage: "Path to SSH private key",
					},
				},
				Action: remoteStatus,
			},
			{
				Name:   "events",
				Usage:  "Monitor Docker events in real time",
				Action: dockerEvents,
			},
			{
				Name:  "watch",
				Usage: "Continuously monitor containers with auto-refresh",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "interval",
						Aliases: []string{"n"},
						Value:   5,
						Usage:   "Refresh interval in seconds",
					},
					&cli.BoolFlag{
						Name:  "no-clear",
						Usage: "Don't clear screen between updates",
					},
				},
				Action: watchMode,
			},
			{
				Name:  "stats",
				Usage: "Display container resource usage statistics",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "no-stream",
						Usage: "Disable streaming stats and only pull the first result",
					},
				},
				Action: showStats,
			},
			{
				Name:  "logs",
				Usage: "Stream logs from a container",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "tail",
						Aliases: []string{"n"},
						Value:   100,
						Usage:   "Number of lines to show from the end of the logs",
					},
					&cli.BoolFlag{
						Name:    "follow",
						Aliases: []string{"f"},
						Usage:   "Follow log output",
					},
				},
				Action: containerLogs,
			},
		},
		Action: fullStatus,
	}

	if err := app.Run(os.Args); err != nil {
		color.Red("❌ Error: %v", err)
		os.Exit(1)
	}
}

// fullStatus displays full local Docker container and service status.
func fullStatus(c *cli.Context) error {
	useJSON := c.Bool("json")
	filter := c.String("filter")
	if !useJSON {
		color.Cyan("Checking local Docker containers and services...")
	}
	return executeLocalDockerStatus(c.Context, []string{}, useJSON, filter)
}

// stateOnly displays only container states.
func stateOnly(c *cli.Context) error {
	useJSON := c.Bool("json")
	filter := c.String("filter")
	if !useJSON {
		color.Cyan("Checking local container states...")
	}
	return executeLocalDockerStatus(c.Context, []string{"--format", "📂 {{.Names}}: 🔹 {{.Status}}"}, useJSON, filter)
}

// serviceOnly checks local service availability.
func serviceOnly(c *cli.Context) error {
	useJSON := c.Bool("json")
	if !useJSON {
		color.Cyan("Checking local service availability...")
	}
	return executeLocalServiceCheck(c.Context, useJSON)
}

// remoteStatus connects to a remote Docker host via SSH.
func remoteStatus(c *cli.Context) error {
	useJSON := c.Bool("json")
	host := c.String("host")
	args := c.Args()

	if host != "" {
		clientConfig, remoteAddress, err := getSSHConfig(host)
		if err != nil {
			return fmt.Errorf("SSH config error for '%s': %v", host, err)
		}
		if !useJSON {
			color.Cyan("Connecting to %s (%s)...", host, remoteAddress)
		}
		return executeRemoteDockerStatus(c.Context, clientConfig, remoteAddress, useJSON)
	} else if args.Len() > 0 {
		userHost := args.Get(0)
		keyPath := c.String("i")
		if keyPath == "" {
			return fmt.Errorf("Missing SSH key (-i <sshkey>)")
		}
		clientConfig, remoteAddress, err := getManualSSHConfig(userHost, keyPath)
		if err != nil {
			return fmt.Errorf("SSH config error for '%s': %v", userHost, err)
		}
		if !useJSON {
			color.Cyan("Connecting to %s with provided SSH key...", remoteAddress)
		}
		return executeRemoteDockerStatus(c.Context, clientConfig, remoteAddress, useJSON)
	}
	return fmt.Errorf("Missing args. Use '--host <alias>' or '<user>@<host> -i <sshkey>'")
}

// dockerEvents subscribes to Docker events in real time.
func dockerEvents(c *cli.Context) error {
	useJSON := c.Bool("json")
	if !useJSON {
		color.Cyan("Subscribing to Docker events... (press Ctrl+C to exit)")
	}

	cliDocker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("Docker client error: %v", err)
	}
	defer cliDocker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	options := types.EventsOptions{}
	msgChan, errChan := cliDocker.Events(ctx, options)

	for {
		select {
		case event := <-msgChan:
			if useJSON {
				data, err := json.Marshal(event)
				if err != nil {
					return fmt.Errorf("JSON marshal error: %v", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Type: %s | Action: %s | Actor: %v | Time: %s\n",
					event.Type,
					event.Action,
					event.Actor.Attributes,
					time.Unix(event.Time, 0).Format(time.RFC3339))
			}
		case err := <-errChan:
			return fmt.Errorf("Event error: %v", err)
		}
	}
}

// executeLocalDockerStatus runs "docker ps" locally.
func executeLocalDockerStatus(ctx context.Context, args []string, useJSON bool, filter string) error {
	baseArgs := []string{"ps"}

	// Add filter if provided
	if filter != "" {
		baseArgs = append(baseArgs, "--filter", filter)
	}

	if useJSON {
		baseArgs = append(baseArgs, "--format", "{{json .}}")
		cmdArgs := append(baseArgs, args...)
		cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker ps failed: %v\n%s", err, string(output))
		}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		jsonArray := "[" + strings.Join(lines, ",") + "]"
		fmt.Println(jsonArray)
		return nil
	}

	baseArgs = append(baseArgs, "--format", "📦 {{.Names}} | 🔹 {{.Status}} | 🔍 {{.Ports}}")
	cmdArgs := append(baseArgs, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker ps failed: %v\n%s", err, string(output))
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		color.Yellow("No running containers found!")
	} else {
		color.Green("Local Containers:")
		fmt.Println(trimmed)
	}
	return nil
}

// executeLocalServiceCheck checks services using HTTP.
func executeLocalServiceCheck(ctx context.Context, useJSON bool) error {
	if !useJSON {
		color.Cyan("Checking services on ports...")
	}
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}: {{.Ports}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to get containers: %v\n%s", err, string(output))
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		if !useJSON {
			color.Yellow("No running containers found!")
		} else {
			fmt.Println("[]")
		}
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []ServiceCheckResult

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ": ")
		if len(parts) != 2 {
			continue
		}
		container := parts[0]
		ports := strings.Split(parts[1], ", ")
		for _, portInfo := range ports {
			portParts := strings.Split(portInfo, "->")
			if len(portParts) != 2 {
				continue
			}
			hostPortParts := strings.Split(portParts[0], ":")
			if len(hostPortParts) < 2 {
				continue
			}
			port := hostPortParts[1]
			url := fmt.Sprintf("http://localhost:%s", port)
			wg.Add(1)
			go func(container, port, url string) {
				defer wg.Done()
				req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
				if err != nil {
					mu.Lock()
					results = append(results, ServiceCheckResult{Container: container, Port: port, Status: "request error"})
					mu.Unlock()
					return
				}
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Do(req)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					results = append(results, ServiceCheckResult{Container: container, Port: port, Status: "unreachable"})
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					results = append(results, ServiceCheckResult{Container: container, Port: port, Status: "available"})
				} else {
					results = append(results, ServiceCheckResult{Container: container, Port: port, Status: fmt.Sprintf("HTTP %d", resp.StatusCode)})
				}
			}(container, port, url)
		}
	}
	wg.Wait()
	if useJSON {
		jsonData, err := json.Marshal(results)
		if err != nil {
			return fmt.Errorf("JSON marshal error: %v", err)
		}
		fmt.Println(string(jsonData))
	} else {
		for _, r := range results {
			switch r.Status {
			case "available":
				color.Green("%s service is available on port %s.", r.Container, r.Port)
			case "unreachable":
				color.Red("%s on port %s is unreachable.", r.Container, r.Port)
			default:
				color.Yellow("%s service returned %s on port %s.", r.Container, r.Status, r.Port)
			}
		}
		color.Green("Service check completed.")
	}
	return nil
}

// executeRemoteDockerStatus runs "docker ps" on a remote host via SSH.
func executeRemoteDockerStatus(ctx context.Context, config *ssh.ClientConfig, addr string, useJSON bool) error {
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("Failed to connect to %s: %v", addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("Session error on %s: %v", addr, err)
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b

	if useJSON {
		err = session.Run("docker ps --format '{{json .}}'")
	} else {
		err = session.Run("docker ps --format '📦 {{.Names}} | 🔹 {{.Status}} | 🔎 {{.Ports}}'")
	}
	if err != nil {
		return fmt.Errorf("docker ps failed on %s: %v", addr, err)
	}

	output := strings.TrimSpace(b.String())
	if output == "" {
		if useJSON {
			fmt.Println("[]")
		} else {
			color.Yellow("No running containers on remote host!")
		}
	} else {
		if useJSON {
			lines := strings.Split(output, "\n")
			jsonArray := "[" + strings.Join(lines, ",") + "]"
			fmt.Println(jsonArray)
		} else {
			color.Green("Remote Containers:")
			fmt.Println(output)
		}
	}
	return nil
}

// getSSHConfig retrieves SSH configuration from ~/.ssh/config using an alias.
func getSSHConfig(alias string) (*ssh.ClientConfig, string, error) {
	sshConfigPath := os.ExpandEnv("$HOME/.ssh/config")
	f, err := os.Open(sshConfigPath)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot open SSH config: %v", err)
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, "", fmt.Errorf("Decode error: %v", err)
	}

	hostname, err := cfg.Get(alias, "HostName")
	if err != nil || hostname == "" {
		return nil, "", fmt.Errorf("HostName not found for %s", alias)
	}

	user, err := cfg.Get(alias, "User")
	if err != nil || user == "" {
		user = os.Getenv("USER")
	}

	keyPath, err := cfg.Get(alias, "IdentityFile")
	if err != nil || keyPath == "" {
		keyPath = os.ExpandEnv("$HOME/.ssh/id_rsa")
	} else {
		keyPath, err = expandPath(keyPath)
		if err != nil {
			return nil, "", fmt.Errorf("Key path error: %v", err)
		}
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot read key at %s: %v", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot parse key at %s: %v", keyPath, err)
	}

	knownHostsFile := os.ExpandEnv("$HOME/.ssh/known_hosts")
	hostKeyCallback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, "", fmt.Errorf("Host key callback error: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	return clientConfig, fmt.Sprintf("%s:22", hostname), nil
}

// getManualSSHConfig retrieves SSH configuration from a user@host string and key path.
func getManualSSHConfig(userHost, keyPath string) (*ssh.ClientConfig, string, error) {
	keyPath, err := expandPath(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("Key path error: %v", err)
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot read key at %s: %v", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("Cannot parse key at %s: %v", keyPath, err)
	}

	parts := strings.Split(userHost, "@")
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("Invalid user@host: %s", userHost)
	}
	user := parts[0]
	host := parts[1]

	knownHostsFile := os.ExpandEnv("$HOME/.ssh/known_hosts")
	hostKeyCallback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, "", fmt.Errorf("Host key callback error: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	return clientConfig, fmt.Sprintf("%s:22", host), nil
}

// expandPath expands the "~" to the home directory.
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return strings.Replace(path, "~", home, 1), nil
	}
	return path, nil
}

// watchMode continuously monitors containers with auto-refresh.
func watchMode(c *cli.Context) error {
	useJSON := c.Bool("json")
	interval := time.Duration(c.Int("interval")) * time.Second
	noClear := c.Bool("no-clear")
	filter := c.String("filter")

	if useJSON {
		return fmt.Errorf("watch mode not supported with JSON output")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Initial display
	if !noClear {
		clearScreen()
	}
	displayStatus(c.Context, filter)

	for {
		select {
		case <-ticker.C:
			if !noClear {
				clearScreen()
			}
			displayStatus(c.Context, filter)
			fmt.Printf("\n🕐 Last update: %s | Refresh: %ds | Press Ctrl+C to exit\n",
				time.Now().Format("15:04:05"), c.Int("interval"))
		case <-sigChan:
			color.Yellow("\n👋 Exiting watch mode...")
			return nil
		}
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func displayStatus(ctx context.Context, filter string) {
	color.Cyan("🐳 Docker Container Monitor - Live View")
	fmt.Println(strings.Repeat("=", 70))
	executeLocalDockerStatus(ctx, []string{}, false, filter)
}

// showStats displays container resource usage statistics.
func showStats(c *cli.Context) error {
	useJSON := c.Bool("json")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("Docker client error: %v", err)
	}
	defer cli.Close()

	ctx := context.Background()

	// List all running containers
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return fmt.Errorf("Failed to list containers: %v", err)
	}

	if len(containers) == 0 {
		color.Yellow("No running containers found!")
		return nil
	}

	var stats []ContainerStat

	for _, container := range containers {
		statsResponse, err := cli.ContainerStats(ctx, container.ID, false)
		if err != nil {
			continue
		}

		var v types.StatsJSON
		if err := json.NewDecoder(statsResponse.Body).Decode(&v); err != nil {
			statsResponse.Body.Close()
			continue
		}
		statsResponse.Body.Close()

		// Calculate CPU percentage
		cpuPercent := calculateCPUPercent(&v)
		memPercent := 0.0
		if v.MemoryStats.Limit > 0 {
			memPercent = float64(v.MemoryStats.Usage) / float64(v.MemoryStats.Limit) * 100.0
		}

		// Calculate network I/O
		var networkRx, networkTx uint64
		for _, network := range v.Networks {
			networkRx += network.RxBytes
			networkTx += network.TxBytes
		}

		name := container.Names[0]
		if strings.HasPrefix(name, "/") {
			name = name[1:]
		}

		stats = append(stats, ContainerStat{
			Name:          name,
			CPUPercent:    cpuPercent,
			MemoryUsage:   v.MemoryStats.Usage,
			MemoryLimit:   v.MemoryStats.Limit,
			MemoryPercent: memPercent,
			NetworkRx:     networkRx,
			NetworkTx:     networkTx,
		})
	}

	if useJSON {
		jsonData, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		printStatsTable(stats)
	}

	return nil
}

func calculateCPUPercent(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	cpuCount := float64(stats.CPUStats.OnlineCPUs)

	if cpuCount == 0 {
		cpuCount = float64(runtime.NumCPU())
	}

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * cpuCount * 100.0
	}
	return 0.0
}

func printStatsTable(stats []ContainerStat) {
	color.Cyan("📊 Container Resource Statistics")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-25s %-10s %-25s %-10s %-20s\n",
		"CONTAINER", "CPU %", "MEMORY USAGE", "MEM %", "NET I/O (RX/TX)")
	fmt.Println(strings.Repeat("-", 100))

	for _, stat := range stats {
		memUsage := formatBytes(stat.MemoryUsage)
		memLimit := formatBytes(stat.MemoryLimit)
		netIO := fmt.Sprintf("%s / %s", formatBytes(stat.NetworkRx), formatBytes(stat.NetworkTx))

		// Color code based on usage
		cpuColor := color.GreenString
		if stat.CPUPercent > 80 {
			cpuColor = color.RedString
		} else if stat.CPUPercent > 50 {
			cpuColor = color.YellowString
		}

		memColor := color.GreenString
		if stat.MemoryPercent > 90 {
			memColor = color.RedString
		} else if stat.MemoryPercent > 70 {
			memColor = color.YellowString
		}

		fmt.Printf("%-25s %s %-25s %s %-20s\n",
			truncate(stat.Name, 25),
			cpuColor("%-10.2f", stat.CPUPercent),
			fmt.Sprintf("%s / %s", memUsage, memLimit),
			memColor("%-10.2f", stat.MemoryPercent),
			netIO)
	}
	fmt.Println(strings.Repeat("=", 100))
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// containerLogs streams logs from a container.
func containerLogs(c *cli.Context) error {
	if c.NArg() == 0 {
		return fmt.Errorf("Please specify a container name or ID")
	}

	containerName := c.Args().Get(0)
	tail := c.Int("tail")
	follow := c.Bool("follow")

	args := []string{"logs"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, containerName)

	cmd := exec.CommandContext(c.Context, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if !follow {
		color.Cyan("📜 Logs from container: %s", containerName)
		fmt.Println(strings.Repeat("=", 70))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to get logs for %s: %v\n💡 Try: docker ps to see available containers", containerName, err)
	}

	return nil
}
