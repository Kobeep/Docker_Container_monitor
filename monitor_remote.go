package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Retrieves SSH configuration for a given host alias
func getSSHConfig(host string) (*ssh.ClientConfig, string, error) {
	// Find the SSH config file
	sshConfigPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	sshKnownHostsPath := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")

	// Parse the SSH config
	configFile, err := os.Open(sshConfigPath)
	if err != nil {
		return nil, "", fmt.Errorf("could not open SSH config file: %v", err)
	}
	defer configFile.Close()

	// Execute `ssh` to get the host information (leverages the existing SSH config)
	cmd := exec.Command("ssh", "-G", host)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to retrieve host config: %v", err)
	}

	// Parse the output from `ssh -G`
	sshArgs := parseSSHConfigOutput(output)
	if sshArgs["hostname"] == "" {
		return nil, "", fmt.Errorf("hostname not found in SSH config")
	}

	// Use parsed values for hostname and port
	hostname := sshArgs["hostname"]
	port := sshArgs["port"]

	// Parse private key from SSH config
	keyPath := sshArgs["identityfile"]
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("could not read private key: %v", err)
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse private key: %v", err)
	}

	// Create the SSH client configuration
	hostKeyCallback, err := knownhosts.New(sshKnownHostsPath)
	if err != nil {
		return nil, "", fmt.Errorf("could not load known hosts file: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User:            sshArgs["user"], // Retrieve username from config
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
	}

	return clientConfig, fmt.Sprintf("%s:%s", hostname, port), nil
}

// Parse the output of `ssh -G <host>` into a map
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
	// Connect to the remote host
	client, err := ssh.Dial("tcp", host, clientConfig)
	if err != nil {
		return "", fmt.Errorf("failed to connect to remote host: %v", err)
	}
	defer client.Close()

	// Create a new session
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Run the command on the remote host
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	err = session.Run(command)
	if err != nil {
		return "", fmt.Errorf("command failed: %v (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

func getRemoteContainers(clientConfig *ssh.ClientConfig, host string) []string {
	command := "docker ps --format '{{.Names}}'"
	output, err := runRemoteCommand(clientConfig, host, command)
	if err != nil {
		log.Printf("❌ Error fetching containers: %v\n", err)
		return nil
	}

	containers := bytes.Split([]byte(output), []byte("\n"))
	var result []string
	for _, container := range containers {
		if len(container) > 0 {
			result = append(result, string(container))
		}
	}
	return result
}

func getRemoteServiceStatus(clientConfig *ssh.ClientConfig, host, container string) string {
	command := fmt.Sprintf("docker inspect -f '{{range $p, $conf := .NetworkSettings.Ports}}{{$p}} {{end}}' %s", container)
	output, err := runRemoteCommand(clientConfig, host, command)
	if err != nil {
		log.Printf("❌ Error fetching service status for %s: %v\n", container, err)
		return "Unknown"
	}
	return output
}

func main() {
	keyPath := flag.String("i", "", "Path to the SSH private key (optional if using SSH config)")
	hostAlias := flag.String("host", "", "Host alias or address (from SSH config or manual)")
	flag.Parse()

	if *hostAlias == "" {
		log.Fatal("❌ Missing required argument: -host <hostalias>")
	}

	var clientConfig *ssh.ClientConfig
	var remoteAddress string
	var err error

	// Use the SSH config if no private key is provided
	if *keyPath == "" {
		clientConfig, remoteAddress, err = getSSHConfig(*hostAlias)
		if err != nil {
			log.Fatalf("❌ Failed to load SSH config: %v", err)
		}
	} else {
		log.Fatalf("Direct key-based implementation is deprecated. Remove .ADD_MISC.ALREADY_FORMED TRUE --")
	}

	fmt.Printf("🔄 Connecting to %s (%s)...\n", *hostAlias, remoteAddress)

	containers := getRemoteContainers(clientConfig, remoteAddress)
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found on the remote host.")
		return
	}

	fmt.Println("📦 Remote Docker Containers:")
	for _, container := range containers {
		serviceStatus := getRemoteServiceStatus(clientConfig, remoteAddress, container)
		fmt.Printf("📌 %s: Service Status - %s\n", container, serviceStatus)
	}
}
