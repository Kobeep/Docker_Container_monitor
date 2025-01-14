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

// Checks if a Docker container is running
func getRunningContainers() []string {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		log.Println("Error fetching running containers:", err)
		return nil
	}
	containers := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(containers) == 1 && containers[0] == "" {
		return nil
	}
	return containers
}

// Gets the exposed ports for a given container
func getContainerPorts(container string) string {
	out, err := exec.Command("docker", "inspect", "-f", "{{range $p, $conf := .NetworkSettings.Ports}}{{$p}} {{end}}", container).Output()
	if err != nil {
		log.Println("Error fetching ports for container:", container, err)
		return "No data"
	}
	return strings.TrimSpace(string(out))
}

// Checks if the service inside the container is available
func checkService(url string) bool {
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
	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	allServicesAvailable := true

	for _, container := range containers {
		portInfo := getContainerPorts(container)

		// Extract first available port (if any)
		portParts := strings.Split(portInfo, "/")
		var url string
		if len(portParts) > 0 && portParts[0] != "" {
			url = fmt.Sprintf("http://localhost:%s", strings.Split(portParts[0], " ")[0])
		} else {
			url = "N/A"
		}

		serviceRunning := url != "N/A" && checkService(url)

		fmt.Printf("📌 %s: Container - 🟢 Running, Ports - %s, Service - %v\n",
			container,
			portInfo,
			map[bool]string{true: "🟢 Available", false: "🔴 Unavailable"}[serviceRunning],
		)

		// If any service is unavailable, mark failure
		if !serviceRunning {
			allServicesAvailable = false
		}
	}

	// If any service is down, print a warning but exit with 0 (pipeline will not fail)
	if !allServicesAvailable {
		fmt.Println("⚠️ Warning: Some services are unavailable, but continuing execution.")
		os.Exit(0) // Prevents CI/CD failure
	}

	return nil
}

// Displays only container state
func stateOnly(c *cli.Context) error {
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
	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	for _, container := range containers {
		portInfo := getContainerPorts(container)

		// Extract first available port
		portParts := strings.Split(portInfo, "/")
		var url string
		if len(portParts) > 0 && portParts[0] != "" {
			url = fmt.Sprintf("http://localhost:%s", strings.Split(portParts[0], " ")[0])
		} else {
			url = "N/A"
		}

		serviceRunning := url != "N/A" && checkService(url)

		fmt.Printf("📌 %s: Service - %v\n",
			container,
			map[bool]string{true: "🟢 Available", false: "🔴 Unavailable"}[serviceRunning],
		)
	}

	return nil
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

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
