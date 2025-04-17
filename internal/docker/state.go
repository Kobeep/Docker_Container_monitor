package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

// StateCmd lists containers with colored statuses
func StateCmd(c *cli.Context) error {
	args := []string{"ps", "--format", "{{.Names}}|{{.Status}}"}
	if c.Bool("json") {
		out, err := exec.Command("docker", "ps", "--format", "{{json .}}").Output()
		if err != nil {
			return fmt.Errorf("docker ps failed: %v", err)
		}
		// wrap lines into JSON array
		arr := strings.Split(strings.TrimSpace(string(out)), "\n")
		fmt.Println("[" + strings.Join(arr, ",") + "]")
		return nil
	}

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker ps failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		color.Yellow("⚠️  No running containers")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"📦 Container", "🔹 Status"})
	table.SetCaption(true, color.GreenString("Containers"))

	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		name, status := parts[0], parts[1]
		var s string
		switch {
		case strings.HasPrefix(status, "Up"):
			s = color.GreenString("✔️  %s", status)
		case strings.HasPrefix(status, "Exited"):
			s = color.RedString("❌  %s", status)
		default:
			s = color.YellowString("⚠️  %s", status)
		}
		table.Append([]string{name, s})
	}
	table.Render()
	return nil
}
