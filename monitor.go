package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
)

// Retrieves a list of running Docker containers
func getRunningContainers() []string {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
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

// Retrieves the exposed ports for a given container
func getContainerPorts(container string) (string, string) {
	out, err := exec.Command("docker", "inspect", "-f", "{{range $p, $conf := .NetworkSettings.Ports}}{{$p}} {{end}}", container).Output()
	if err != nil {
		log.Println("❌ Error fetching ports for container:", container, err)
		return "N/A", "N/A"
	}
	ports := strings.Fields(strings.TrimSpace(string(out))) // Splitting safely

	if len(ports) > 0 {
		// Extract host and container ports
		portMappingOut, _ := exec.Command("docker", "port", container).Output()
		portMapping := strings.Split(strings.TrimSpace(string(portMappingOut)), "\n")

		if len(portMapping) > 0 {
			portParts := strings.Fields(portMapping[0]) // Example: "80/tcp -> 0.0.0.0:8081"
			if len(portParts) > 2 {
				hostPort := strings.Split(portParts[2], ":")[1] // Extract 8081
				return ports[0], hostPort
			}
		}
		return ports[0], "Unknown"
	}

	return "N/A", "N/A"
}

// Checks if the service inside the container is available
func checkService(port string) bool {
	url := fmt.Sprintf("http://localhost:%s", port)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// Displays full container and service status
func fullStatus(c *cli.Context) error {
	// Separator line before printing new results
	fmt.Println("\n--------------------------------------")
	fmt.Println("🔄 Checking container and service status...")
	fmt.Println("--------------------------------------\n")

	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	for _, container := range containers {
		containerPort, hostPort := getContainerPorts(container)
		serviceRunning := containerPort != "N/A" && checkService(hostPort)

		fmt.Printf("📌 %s: Container - 🟢 Running, Service Port - %s, Host Port - %s, Service - %v\n",
			container,
			containerPort,
			hostPort,
			map[bool]string{true: "🟢 Available", false: "🔴 Unavailable"}[serviceRunning],
		)
	}

	return nil
}

// Displays only container state
func stateOnly(c *cli.Context) error {
	fmt.Println("\n--------------------------------------")
	fmt.Println("🔄 Checking container states...")
	fmt.Println("--------------------------------------\n")

	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	for _, container := range containers {
		fmt.Printf("📌 %s: 🟢 Running\n", container)
	}
	return nil
}

// Displays only service availability
func serviceOnly(c *cli.Context) error {
	fmt.Println("\n--------------------------------------")
	fmt.Println("🔄 Checking service availability...")
	fmt.Println("--------------------------------------\n")

	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	for _, container := range containers {
		containerPort, hostPort := getContainerPorts(container)
		serviceRunning := containerPort != "N/A" && checkService(hostPort)

		fmt.Printf("📌 %s: Service Port - %s, Host Port - %s, Service - %v\n",
			container,
			containerPort,
			hostPort,
			map[bool]string{true: "🟢 Available", false: "🔴 Unavailable"}[serviceRunning],
		)
	}

	return nil
}

// Runs the monitor tool as a continuous systemd service
func runAsService() {
	fmt.Println("🚀 Starting Monitor Service...")

	for {
		fullStatus(nil)
		time.Sleep(10 * time.Second)
	}
}

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
		},
		Action: fullStatus,
	}

	// If no arguments are given, assume it's running as a systemd service
	if len(os.Args) > 1 {
		err := app.Run(os.Args)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		runAsService()
	}
}
