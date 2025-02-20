package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
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

func main() {
	app := &cli.App{
		Name:  "monitor",
		Usage: "Monitor Docker containers, services and events (local and remote)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output in JSON format",
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
	if !useJSON {
		color.Cyan("Checking local Docker containers and services...")
	}
	return executeLocalDockerStatus(c.Context, []string{}, useJSON)
}

// stateOnly displays only container states.
func stateOnly(c *cli.Context) error {
	useJSON := c.Bool("json")
	if !useJSON {
		color.Cyan("Checking local container states...")
	}
	return executeLocalDockerStatus(c.Context, []string{"--format", "📂 {{.Names}}: 🔹 {{.Status}}"}, useJSON)
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
func executeLocalDockerStatus(ctx context.Context, args []string, useJSON bool) error {
	if useJSON {
		baseArgs := []string{"ps", "--format", "{{json .}}"}
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

	baseArgs := []string{"ps", "--format", "📦 {{.Names}} | 🔹 {{.Status}} | 🔍 {{.Ports}}"}
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
