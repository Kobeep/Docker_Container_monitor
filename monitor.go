package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"os/user"

	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Global Docker host (default is local)
var dockerHost = "unix:///var/run/docker.sock"

// Expand `~` to the user's home directory
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		usr, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("failed to get current user: %v", err)
		}
		return filepath.Join(usr.HomeDir, path[2:]), nil
	}
	return path, nil
}

// Fetch running containers
func getRunningContainers() []string {
	cmd := exec.Command("docker", "-H", dockerHost, "ps", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		log.Println("❌ Error fetching running containers:", err)
		return nil
	}
	containers := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(containers) == 1 && containers[0] == "" {
		return nil
	}
	return containers
}

// Fetch exposed ports for a container
func getContainerPorts(container string) string {
	cmd := exec.Command("docker", "-H", dockerHost, "inspect", "-f", "{{range $p, $conf := .NetworkSettings.Ports}}{{$p}} {{end}}", container)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("❌ Error fetching ports for container %s: %v", container, err)
		return "Unknown"
	}
	return strings.TrimSpace(string(out))
}

// Display state-only information
func stateOnly(c *cli.Context) error {
	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	fmt.Println("📦 Running Containers:")
	for _, container := range containers {
		fmt.Printf("📌 %s: 🟢 Running\n", container)
	}
	return nil
}

// Display service-only information
func serviceOnly(c *cli.Context) error {
	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	fmt.Println("🔧 Service Status:")
	for _, container := range containers {
		ports := getContainerPorts(container)
		fmt.Printf("📌 %s: Ports - %s, Service - 🟢 Available\n", container, ports)
	}
	return nil
}

// Display full status information
func fullStatus(c *cli.Context) error {
	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	fmt.Println("📊 Full Container and Service Status:")
	for _, container := range containers {
		ports := getContainerPorts(container)
		fmt.Printf("📌 %s: 🟢 Running, Ports - %s, Service - 🟢 Available\n", container, ports)
	}
	return nil
}

// Fetch SSH configuration for a host
func getSSHConfig(host string) (*ssh.ClientConfig, string, error) {
	sshKnownHostsPath := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")

	cmd := exec.Command("ssh", "-G", host)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to retrieve host config: %v", err)
	}

	sshArgs := parseSSHConfigOutput(output)
	if sshArgs["hostname"] == "" {
		return nil, "", fmt.Errorf("hostname not found in SSH config")
	}

	hostname := sshArgs["hostname"]
	port := sshArgs["port"]

	keyPath, err := expandPath(sshArgs["identityfile"])
	if err != nil {
		return nil, "", fmt.Errorf("could not expand identity file path: %v", err)
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("could not read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse private key: %v", err)
	}

	hostKeyCallback, err := knownhosts.New(sshKnownHostsPath)
	if err != nil {
		return nil, "", fmt.Errorf("could not load known hosts file: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User:            sshArgs["user"],
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
	}

	return clientConfig, fmt.Sprintf("%s:%s", hostname, port), nil
}

// Parse SSH config output
func parseSSHConfigOutput(output []byte) map[string]string {
	lines := bytes.Split(output, []byte("\n"))
	config := make(map[string]string)
	for _, line := range lines {
		parts := bytes.Fields(line)
		if len(parts) == 2 {
			config[string(parts[0])] = string(parts[1])
		}
	}
	return config
}

// Execute a command on a remote host
func runRemoteCommand(clientConfig *ssh.ClientConfig, host string, command string) (string, error) {
	client, err := ssh.Dial("tcp", host, clientConfig)
	if err != nil {
		return "", fmt.Errorf("failed to connect to remote host: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	err = session.Run(command)
	if err != nil {
		return "", fmt.Errorf("command failed: %v (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

// Fetch ports for a container on a remote host
func getRemoteContainerPorts(clientConfig *ssh.ClientConfig, host, container string) string {
	command := fmt.Sprintf("docker inspect -f '{{range $p, $conf := .NetworkSettings.Ports}}{{$p}} {{end}}' %s", container)
	output, err := runRemoteCommand(clientConfig, host, command)
	if err != nil {
		log.Printf("❌ Error fetching ports for container %s on remote host: %v", container, err)
		return "Unknown"
	}
	return strings.TrimSpace(output)
}

// Display remote status
func remoteStatus(c *cli.Context) error {
	host := c.String("host")
	if host == "" {
		log.Fatal("❌ Missing required argument: --host <hostalias>")
	}

	clientConfig, remoteAddress, err := getSSHConfig(host)
	if err != nil {
		log.Fatalf("❌ Failed to load SSH config: %v", err)
	}

	fmt.Printf("🔄 Connecting to %s (%s)...\n", host, remoteAddress)

	command := "docker ps --format '{{.Names}}'"
	output, err := runRemoteCommand(clientConfig, remoteAddress, command)
	if err != nil {
		log.Printf("❌ Error fetching containers: %v\n", err)
		return nil
	}

	containers := strings.Split(strings.TrimSpace(output), "\n")
	fmt.Println("📦 Remote Docker Containers:")
	for _, container := range containers {
		if container != "" {
			ports := getRemoteContainerPorts(clientConfig, remoteAddress, container)
			fmt.Printf("📌 %s: 🟢 Running, Ports - %s, Service - 🟢 Available\n", container, ports)
		}
	}

	return nil
}

func main() {
	app := &cli.App{
		Name:  "monitor",
		Usage: "Monitor running Docker containers and their services",
		Description: `The monitor CLI tool allows you to check the status of local and remote Docker containers.
It includes the following modes:

- "state"   : Displays only the container names and their running states.
- "service" : Displays only the status of services running inside the containers.
- "remote"  : Allows monitoring containers on a remote Docker host via SSH.

Examples:
- Full local status: monitor
- Local container states: monitor state
- Local service status: monitor service
- Remote container status: monitor remote --host <hostalias>

The 'remote' command uses the SSH configuration from '~/.ssh/config' or accepts manual key-based authentication with '--host'.`,
		Commands: []*cli.Command{
			{
				Name:   "state",
				Usage:  "Displays only container names and their states",
				Action: stateOnly,
			},
			{
				Name:   "service",
				Usage:  "Displays only the status of services",
				Action: serviceOnly,
			},
			{
				Name:  "remote",
				Usage: "Monitor Docker containers on a remote host via SSH",
				Description: `The remote command connects to a Docker host over SSH and retrieves the status of its containers.
It uses the SSH configuration from '~/.ssh/config' by default. You can specify a host alias or IP address.

Examples:
- monitor remote --host <hostalias>
- monitor remote --host user@192.168.1.10`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "host",
						Usage: "Host alias or address (from SSH config or manual)",
					},
				},
				Action: remoteStatus,
			},
		},
		Action: fullStatus,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
