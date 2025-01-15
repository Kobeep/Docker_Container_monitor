package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Global Docker host (default is local)
var dockerHost = "unix:///var/run/docker.sock"

// Local Monitoring Functions (unchanged from your current implementation)
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

// Remote Monitoring Functions
func getSSHConfig(host string) (*ssh.ClientConfig, string, error) {
	sshConfigPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
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
	keyPath := sshArgs["identityfile"]
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
			fmt.Printf("📌 %s: 🟢 Running\n", container)
		}
	}

	return nil
}

// Main Application with CLI Commands
func main() {
	app := &cli.App{
		Name:  "monitor",
		Usage: "Monitor running Docker containers and their services",
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
