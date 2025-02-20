package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/fatih/color"
	"github.com/kevinburke/ssh_config"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "monitor",
		Usage: "Monitor Docker containers and services (local and remote)",
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
		},
		Action: fullStatus,
	}

	if err := app.Run(os.Args); err != nil {
		color.Red("App error: %v", err)
		os.Exit(1)
	}
}

// Full local status
func fullStatus(c *cli.Context) error {
	color.Cyan("Checking local Docker containers and services...")
	return executeLocalDockerStatus(c.Context, []string{})
}

// Local container states
func stateOnly(c *cli.Context) error {
	color.Cyan("Checking local container states...")
	return executeLocalDockerStatus(c.Context, []string{"--format", "📂 {{.Names}}: 🔹 {{.Status}}"})
}

// Local service check
func serviceOnly(c *cli.Context) error {
	color.Cyan("Checking local service availability...")
	return executeLocalServiceCheck(c.Context)
}

// Remote status via SSH
func remoteStatus(c *cli.Context) error {
	host := c.String("host")
	args := c.Args()

	if host != "" {
		clientConfig, remoteAddress, err := getSSHConfig(host)
		if err != nil {
			return fmt.Errorf("SSH config error for '%s': %v", host, err)
		}
		color.Cyan("Connecting to %s (%s)...", host, remoteAddress)
		return executeRemoteDockerStatus(c.Context, clientConfig, remoteAddress)
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
		color.Cyan("Connecting to %s with provided SSH key...", remoteAddress)
		return executeRemoteDockerStatus(c.Context, clientConfig, remoteAddress)
	}
	return fmt.Errorf("Missing args. Use '--host <alias>' or '<user>@<host> -i <sshkey>'")
}

// Run docker ps locally
func executeLocalDockerStatus(ctx context.Context, args []string) error {
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

// Check services concurrently
func executeLocalServiceCheck(ctx context.Context) error {
	color.Cyan("Checking services on ports...")
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}: {{.Ports}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to get containers: %v\n%s", err, string(output))
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		color.Yellow("No running containers found!")
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

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
			hostPort := hostPortParts[1]
			url := fmt.Sprintf("http://localhost:%s", hostPort)
			wg.Add(1)
			go func(container, port, url string) {
				defer wg.Done()
				curlCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				curlCmd := exec.CommandContext(curlCtx, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", url)
				out, err := curlCmd.Output()
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					color.Red("%s on port %s is unreachable.", container, port)
				} else {
					code := strings.TrimSpace(string(out))
					if code == "200" {
						color.Green("%s service is available on port %s.", container, port)
					} else {
						color.Yellow("%s service returned HTTP %s on port %s.", container, code, port)
					}
				}
			}(container, hostPort, url)
		}
	}
	wg.Wait()
	color.Green("Service check completed.")
	return nil
}

// Run docker ps on remote host
func executeRemoteDockerStatus(ctx context.Context, config *ssh.ClientConfig, addr string) error {
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
	if err := session.Run("docker ps --format '📦 {{.Names}} | 🔹 {{.Status}} | 🔎 {{.Ports}}'"); err != nil {
		return fmt.Errorf("docker ps failed on %s: %v", addr, err)
	}
	trimmed := strings.TrimSpace(b.String())
	if trimmed == "" {
		color.Yellow("No running containers on remote host!")
	} else {
		color.Green("Remote Containers:")
		fmt.Println(trimmed)
	}
	return nil
}

// Get SSH config from ~/.ssh/config
func getSSHConfig(alias string) (*ssh.ClientConfig, string, error) {
	path := os.ExpandEnv("$HOME/.ssh/config")
	f, err := os.Open(path)
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

// Get manual SSH config
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

// Expand ~ in path
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
