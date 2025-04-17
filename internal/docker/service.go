package docker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

type ServiceCheckResult struct {
	Container string `json:"container"`
	Port      string `json:"port"`
	Status    string `json:"status"`
}

// ServiceCmd performs HTTP checks and shows a table
func ServiceCmd(c *cli.Context) error {
	threshold := c.Duration("threshold")
	webhook := c.String("alert")

	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}: {{.Ports}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker ps failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var result []ServiceCheckResult

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		name, portsStr := parts[0], parts[1]
		for _, p := range strings.Split(portsStr, ", ") {
			hostPort := strings.SplitN(p, "->", 2)[0]
			port := strings.Split(hostPort, ":")[1]
			url := fmt.Sprintf("http://localhost:%s", port)
			wg.Add(1)
			go func(container, port, url string) {
				defer wg.Done()
				start := time.Now()
				resp, err := http.Get(url)
				status := "unreachable"
				if err == nil {
					if resp.StatusCode == 200 && time.Since(start) < threshold {
						status = "available"
					} else {
						status = fmt.Sprintf("%s (%.0fms)", resp.Status, time.Since(start).Seconds()*1000)
					}
					resp.Body.Close()
				}
				mu.Lock()
				result = append(result, ServiceCheckResult{container, port, status})
				mu.Unlock()
			}(name, port, url)
		}
	}
	wg.Wait()

	if c.Bool("json") {
		b, _ := json.Marshal(result)
		fmt.Println(string(b))
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Container", "Port", "Status"})
	table.SetCaption(true, color.MagentaString("Service checks"))

	for _, r := range result {
		var s string
		switch r.Status {
		case "available":
			s = color.GreenString("✔️ %s", r.Status)
		case "unreachable":
			s = color.RedString("❌ %s", r.Status)
		default:
			s = color.YellowString("⚠️ %s", r.Status)
		}
		table.Append([]string{r.Container, r.Port, s})
		if webhook != "" && r.Status != "available" {
			// fire webhook
			http.Post(webhook, "application/json",
				strings.NewReader(fmt.Sprintf(`{"container":"%s","port":"%s","status":"%s"}`, r.Container, r.Port, r.Status)))
		}
	}
	table.Render()
	return nil
}
