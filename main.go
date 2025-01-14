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

// Retrieves the list of running containers
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

// Retrieves container ports
func getContainerPorts(container string) string {
	out, err := exec.Command("docker", "inspect", "-f", "{{range $p, $conf := .NetworkSettings.Ports}}{{$p}} {{end}}", container).Output()
	if err != nil {
		log.Println("Error fetching ports for container:", container, err)
		return "No data"
	}
	return strings.TrimSpace(string(out))
}

// Checks if the service is available
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

	for _, container := range containers {
		portInfo := getContainerPorts(container)
		url := fmt.Sprintf("http://localhost:%s", strings.Split(portInfo, "/")[0])
		serviceRunning := checkService(url)

		fmt.Printf("📌 %s: Container - 🟢 Running, Ports - %s, Service - %v\n",
			container,
			portInfo,
			map[bool]string{true: "🟢 Available", false: "🔴 Unavailable"}[serviceRunning],
		)
	}
	return nil
}

// Displays only container status
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

// Displays only service status
func serviceOnly(c *cli.Context) error {
	containers := getRunningContainers()
	if len(containers) == 0 {
		fmt.Println("❌ No running containers found!")
		return nil
	}

	for _, container := range containers {
		portInfo := getContainerPorts(container)
		url := fmt.Sprintf("http://localhost:%s", strings.Split(portInfo, "/")[0])
		serviceRunning := checkService(url)

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
