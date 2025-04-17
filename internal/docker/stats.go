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

// StatsCmd shows container stats in a table
func StatsCmd(c *cli.Context) error {
	rawArgs := []string{"stats", "--no-stream", "--format", "{{json .}}"}
	prettyArgs := []string{"stats", "--no-stream", "--format",
		"⚙️ {{.Name}}|{{.CPUPerc}}|🧠 {{.MemUsage}}|💾 {{.BlockIO}}|🌐 {{.NetIO}}"}
	args := prettyArgs
	if c.Bool("json") {
		args = rawArgs
	}
	args = append(args, c.Args().Slice()...)

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stats failed: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if c.Bool("json") {
		fmt.Println("[" + strings.Join(lines, ",") + "]")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Container", "CPU %", "Memory", "Disk I/O", "Net I/O"})
	table.SetCaption(true, color.CyanString("Live stats"))

	for _, ln := range lines {
		parts := strings.Split(ln, "|")
		if len(parts) != 5 {
			continue
		}
		table.Append([]string{
			strings.TrimPrefix(parts[0], "⚙️ "), parts[1],
			strings.TrimPrefix(parts[2], "🧠 "), strings.TrimPrefix(parts[3], "💾 "),
			strings.TrimPrefix(parts[4], "🌐 "),
		})
	}
	table.Render()
	return nil
}
