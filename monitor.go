package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"
	"github.com/kevinburke/ssh_config"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "monitor",
		Usage: "🔍 Monitor running Docker containers and their services (local and remote)",
		Commands: []*cli.Command{
			{
				Name:   "state",
				Usage:  "📊 Displays only container names and their states",
				Action: stateOnly,
			},
			{
				Name:   "service",
				Usage:  "🛑 Displays only the status of services",
				Action: serviceOnly,
			},
			{
				Name:  "remote",
				Usage: "🚀 Monitor Docker containers on a remote host via SSH",
				Description: `The remote command connects to a Docker host over SSH and retrieves the status of its containers.
You can either:
- 🔹 Use SSH config: monitor remote --host <hostalias>
- 🔹 Use manual details: monitor remote <user>@<hostaddress> -i <sshkey>`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "host",
						Usage: "🏠 Host alias from SSH config",
					},
					&cli.StringFlag{
						Name:  "i",
						Usage: "🔐 Path to the SSH private key (used with manual host authentication)",
					},
				},
				Action: remoteStatus,
			},
		},
		Action: fullStatus,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("❌ Application error: %v", err)
	}
}

// Display full local status
func fullStatus(c *cli.Context) error {
	fmt.Println("🔁 Checking local Docker containers and services...")
	return executeLocalDockerStatus()
}

// Display only container states (local)
func stateOnly(c *cli.Context) error {
	fmt.Println("🔍 Checking local container states...")
	return executeLocalDockerStatus("--format", "📂 {{.Names}}: 🔹 {{.Status}}")
}

// Display only service status (local)
func serviceOnly(c *cli.Context) error {
	fmt.Println("🛑 Checking local service availability...")
	return executeLocalServiceCheck()
}

// Display remote status
func remoteStatus(c *cli.Context) error {
	host := c.String("host")
	args := c.Args()

	if host != "" {
		// Case 1: Using SSH Config with --host
		clientConfig, remoteAddress, err := getSSHConfig(host)
		if err != nil {
			log.Fatalf("❌ Failed to load SSH config for host '%s': %v", host, err)
		}

		fmt.Printf("🚀 Connecting to %s (%s)...\n", host, remoteAddress)
		return executeRemoteDockerStatus(clientConfig, remoteAddress)
	} else if args.Len() > 0 {
		// Case 2: Manual SSH Details with user@host and -i key
		userHost := args.Get(0)
		keyPath := c.String("i")

		if keyPath == "" {
			log.Fatal("❌ Missing required SSH key. Use -i <sshkey> to specify the key.")
		}

		clientConfig, remoteAddress, err := getManualSSHConfig(userHost, keyPath)
		if err != nil {
			log.Fatalf("❌ Failed to create SSH config for '%s': %v", userHost, err)
		}

		fmt.Printf("🚀 Connecting to %s using provided SSH key...\n", remoteAddress)
		return executeRemoteDockerStatus(clientConfig, remoteAddress)
	}

	log.Fatal("❌ Missing required arguments. Use '--host <hostalias>' or '<user>@<hostaddress> -i <sshkey>'.")
	return nil
}

// Executes docker status locally
func executeLocalDockerStatus(args ...string) error {
	// Create the docker ps command with the specified format
	cmd := exec.Command("docker", append([]string{"ps", "--format", "📦 {{.Names}} | 🔹 {{.Status}} | 🔍 {{.Ports}}"}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("❌ docker ps failed: %v\n%s", err, string(output))
	}
	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput == "" {
		// If the result is empty, inform that no running containers were found
		fmt.Println("❌ No running containers found!")
	} else {
		fmt.Printf("📦 Local Containers:\n%s\n", trimmedOutput)
	}
	return nil
}

// Checks local service availability
func executeLocalServiceCheck() error {
	fmt.Println("🔍 Checking services on ports...")

	// Retrieve the list of running containers and their ports
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}: {{.Ports}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("❌ Failed to retrieve running containers: %v\n%s", err, string(output))
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ": ")
		if len(parts) != 2 {
			continue
		}

		containerName := parts[0]
		ports := strings.Split(parts[1], ", ")

		for _, portInfo := range ports {
			portParts := strings.Split(portInfo, "->")
			if len(portParts) != 2 {
				continue
			}

			// Extract the host port from the first part of portInfo
			hostPortParts := strings.Split(portParts[0], ":")
			if len(hostPortParts) < 2 {
				continue
			}
			hostPort := hostPortParts[1]
			serviceURL := fmt.Sprintf("http://localhost:%s", hostPort)

			// Check service availability using curl
			curlCmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", serviceURL)
			curlOutput, err := curlCmd.Output()
			if err != nil {
				fmt.Printf("❌ %s on port %s is unreachable.\n", containerName, hostPort)
			} else {
				statusCode := strings.TrimSpace(string(curlOutput))
				if statusCode == "200" {
					fmt.Printf("✅ %s service is available on port %s.\n", containerName, hostPort)
				} else {
					fmt.Printf("⚠️ %s service returned HTTP %s on port %s.\n", containerName, statusCode, hostPort)
				}
			}
		}
	}

	fmt.Println("✅ Service check completed successfully.")
	return nil
}

// Execute docker status on a remote host
func executeRemoteDockerStatus(config *ssh.ClientConfig, remoteAddress string) error {
	client, err := ssh.Dial("tcp", remoteAddress, config)
	if err != nil {
		return fmt.Errorf("❌ Failed to connect to %s: %v", remoteAddress, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("❌ Failed to create session on %s: %v", remoteAddress, err)
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run("docker ps --format '📦 {{.Names}} | 🔹 {{.Status}} | 🔎 {{.Ports}}'"); err != nil {
		return fmt.Errorf("❌ Failed to run docker ps on %s: %v", remoteAddress, err)
	}

	trimmedOutput := strings.TrimSpace(b.String())
	if trimmedOutput == "" {
		// If the output is empty, inform that no running containers were found on the remote host
		fmt.Println("❌ No running containers found on remote host!")
	} else {
		fmt.Printf("📦 Remote Containers:\n%s\n", trimmedOutput)
	}
	return nil
}

// Fetch SSH configuration using alias from ~/.ssh/config
func getSSHConfig(alias string) (*ssh.ClientConfig, string, error) {
	sshConfigPath := os.ExpandEnv("$HOME/.ssh/config")
	configFile, err := os.Open(sshConfigPath)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Could not open SSH config file: %v", err)
	}
	defer configFile.Close()

	cfg, err := ssh_config.Decode(configFile)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Failed to decode SSH config: %v", err)
	}

	hostname, err := cfg.Get(alias, "HostName")
	if err != nil || hostname == "" {
		return nil, "", fmt.Errorf("❌ Could not find HostName for %s in SSH config", alias)
	}

	user, err := cfg.Get(alias, "User")
	if err != nil || user == "" {
		user = os.Getenv("USER") // Default to current user if not specified
	}

	keyPath, err := cfg.Get(alias, "IdentityFile")
	if err != nil || keyPath == "" {
		keyPath = os.ExpandEnv("$HOME/.ssh/id_rsa") // Default key
	} else {
		keyPath, err = expandPath(keyPath)
		if err != nil {
			return nil, "", fmt.Errorf("❌ Could not expand key path: %v", err)
		}
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Could not read SSH private key at %s: %v", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Could not parse private key at %s: %v", keyPath, err)
	}

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return clientConfig, fmt.Sprintf("%s:22", hostname), nil
}

// Fetch SSH configuration manually using user@host and a private key
func getManualSSHConfig(userHost, keyPath string) (*ssh.ClientConfig, string, error) {
	keyPath, err := expandPath(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Could not expand key path: %v", err)
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Could not read SSH private key at %s: %v", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("❌ Could not parse private key at %s: %v", keyPath, err)
	}

	// Split user and host (format: user@host)
	parts := strings.Split(userHost, "@")
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("❌ Invalid format for user@host: %s", userHost)
	}
	user := parts[0]
	host := parts[1]

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return clientConfig, fmt.Sprintf("%s:22", host), nil
}

// Expand ~ to the home directory
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
