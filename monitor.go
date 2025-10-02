package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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

type ContainerStat struct {
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemUsage    uint64  `json:"mem_usage"`
	MemLimit    uint64  `json:"mem_limit"`
	MemPercent  float64 `json:"mem_percent"`
	NetInput    uint64  `json:"net_input"`
	NetOutput   uint64  `json:"net_output"`
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
				Name:  "filter",
				Usage: "Filter containers (e.g., 'name=nginx', 'status=running', 'label=env=prod')",
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
				Name:   "watch",
				Usage:  "Continuously monitor containers with auto-refresh",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "interval",
						Usage: "Refresh interval in seconds",
						Value: 2,
					},
				},
				Action: watchMode,
			},
			{
				Name:   "stats",
				Usage:  "Show container resource statistics (CPU, memory, network)",
				Action: showStats,
			},
			{
				Name:      "logs",
				Usage:     "Stream container logs",
				ArgsUsage: "<container-name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "follow",
						Usage: "Follow log output",
						Aliases: []string{"f"},
					},
					&cli.IntFlag{
						Name:  "tail",
						Usage: "Number of lines to show from the end",
						Value: 100,
					},
				},
				Action: containerLogs,
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
		},
		Action: fullStatus,
	}

	if err := app.Run(os.Args); err != nil {
		color.Red("App error: %v", err)
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

	// Apply filter if provided
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

// watchMode continuously monitors containers with auto-refresh
func watchMode(c *cli.Context) error {
	interval := c.Int("interval")
	if interval < 1 {
		interval = 2
	}

	color.Cyan("🔄 Watch Mode - Refreshing every %d seconds (Press Ctrl+C to exit)", interval)

	// Setup signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Display immediately
	clearScreen()
	executeLocalDockerStatus(c.Context, []string{}, false, c.String("filter"))

	for {
		select {
		case <-ticker.C:
			clearScreen()
			executeLocalDockerStatus(c.Context, []string{}, false, c.String("filter"))
		case <-sigChan:
			color.Yellow("\n👋 Exiting watch mode...")
			return nil
		}
	}
}

// clearScreen clears the terminal screen
func clearScreen() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J")
	}
}

// showStats displays container resource statistics
func showStats(c *cli.Context) error {
	color.Cyan("📊 Fetching container statistics...")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("Docker client error: %v", err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(c.Context, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("Failed to list containers: %v", err)
	}

	if len(containers) == 0 {
		color.Yellow("No running containers found!")
		return nil
	}

	var stats []ContainerStat
	for _, cont := range containers {
		resp, err := cli.ContainerStats(c.Context, cont.ID, false)
		if err != nil {
			color.Yellow("Warning: Could not get stats for %s", cont.Names[0])
			continue
		}

		var v types.StatsJSON
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Calculate CPU percentage
		cpuPercent := calculateCPUPercent(&v)

		// Memory stats
		memUsage := v.MemoryStats.Usage
		memLimit := v.MemoryStats.Limit
		memPercent := float64(memUsage) / float64(memLimit) * 100.0

		// Network stats
		var netInput, netOutput uint64
		for _, netStats := range v.Networks {
			netInput += netStats.RxBytes
			netOutput += netStats.TxBytes
		}

		containerName := strings.TrimPrefix(cont.Names[0], "/")
		stats = append(stats, ContainerStat{
			Name:       containerName,
			CPUPercent: cpuPercent,
			MemUsage:   memUsage,
			MemLimit:   memLimit,
			MemPercent: memPercent,
			NetInput:   netInput,
			NetOutput:  netOutput,
		})
	}

	if c.Bool("json") {
		jsonData, err := json.Marshal(stats)
		if err != nil {
			return fmt.Errorf("JSON marshal error: %v", err)
		}
		fmt.Println(string(jsonData))
	} else {
		printStatsTable(stats)
	}

	return nil
}

// calculateCPUPercent calculates the CPU usage percentage
func calculateCPUPercent(v *types.StatsJSON) float64 {
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return 0.0
}

// printStatsTable displays statistics in a formatted table
func printStatsTable(stats []ContainerStat) {
	fmt.Println()
	color.Cyan("┌─────────────────────────────────────────────────────────────────────────────┐")
	color.Cyan("│                          CONTAINER STATISTICS                               │")
	color.Cyan("├─────────────────────────────────────────────────────────────────────────────┤")

	// Header
	fmt.Printf("│ %-20s │ %8s │ %15s │ %20s │\n", "CONTAINER", "CPU %", "MEMORY", "NETWORK I/O")
	color.Cyan("├─────────────────────────────────────────────────────────────────────────────┤")

	for _, s := range stats {
		name := truncate(s.Name, 20)
		cpuStr := fmt.Sprintf("%.2f%%", s.CPUPercent)
		memStr := fmt.Sprintf("%s / %s", formatBytes(s.MemUsage), formatBytes(s.MemLimit))
		netStr := fmt.Sprintf("%s / %s", formatBytes(s.NetInput), formatBytes(s.NetOutput))

		// Color coding based on resource usage
		cpuColor := color.New(color.FgGreen)
		if s.CPUPercent > 80 {
			cpuColor = color.New(color.FgRed, color.Bold)
		} else if s.CPUPercent > 50 {
			cpuColor = color.New(color.FgYellow)
		}

		memColor := color.New(color.FgGreen)
		if s.MemPercent > 80 {
			memColor = color.New(color.FgRed, color.Bold)
		} else if s.MemPercent > 50 {
			memColor = color.New(color.FgYellow)
		}

		fmt.Printf("│ %-20s │ ", name)
		cpuColor.Printf("%8s", cpuStr)
		fmt.Printf(" │ ")
		memColor.Printf("%15s", memStr)
		fmt.Printf(" │ %20s │\n", netStr)
	}

	color.Cyan("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

// formatBytes converts bytes to human-readable format
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
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncate truncates a string to the specified length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// containerLogs streams logs from a container
func containerLogs(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("Container name required. Usage: monitor logs <container-name>")
	}

	containerName := c.Args().Get(0)
	follow := c.Bool("follow")
	tailLines := strconv.Itoa(c.Int("tail"))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("Docker client error: %v", err)
	}
	defer cli.Close()

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tailLines,
		Timestamps: true,
	}

	out, err := cli.ContainerLogs(c.Context, containerName, options)
	if err != nil {
		return fmt.Errorf("Failed to get logs for %s: %v", containerName, err)
	}
	defer out.Close()

	if follow {
		color.Cyan("📜 Following logs for %s (Press Ctrl+C to exit)...", containerName)
	} else {
		color.Cyan("📜 Logs for %s (last %s lines):", containerName, tailLines)
	}
	fmt.Println()

	_, err = io.Copy(os.Stdout, out)
	return err
}
