package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/urfave/cli/v2"
)

func StatsCmd(c *cli.Context) error {
	containers := c.Args().Slice()
	formatRaw := []string{"stats", "--no-stream", "--format", "{{json .}}"}
	formatPretty := []string{"stats", "--no-stream", "--format",
		"📊 {{.Name}} | CPU: {{.CPUPerc}} | MEM: {{.MemUsage}}"}

	args := formatPretty
	if c.Bool("json") {
		args = formatRaw
	}
	args = append(args, containers...)

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stats failed: %v\n%s", err, out)
	}
	if c.Bool("json") {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		fmt.Println("[" + strings.Join(lines, ",") + "]")
	} else {
		fmt.Println(string(out))
	}
	return nil
}

func StreamStats(ctx context.Context, cli *client.Client, containers []string, interval time.Duration, useJSON bool) error {
	for {
		for _, id := range containers {
			resp, err := cli.ContainerStatsOneShot(ctx, id)
			if err != nil {
				fmt.Println(err)
				continue
			}
			var stats types.StatsJSON
			json.NewDecoder(resp.Body).Decode(&stats)
			if useJSON {
				b, _ := json.Marshal(stats)
				fmt.Println(string(b))
			} else {
				fmt.Printf("%s: CPU %.2f%% MEM %.2fMiB\n", id, calcCPU(stats), float64(stats.MemoryStats.Usage)/1024/1024)
			}
			resp.Body.Close()
		}
		if !useJSON {
			time.Sleep(interval)
		} else {
			break
		}
	}
	return nil
}

func calcCPU(s types.StatsJSON) float64 {
	delta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	system := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)
	if system > 0 && delta > 0 {
		return (delta / system) * float64(len(s.CPUStats.CPUUsage.PercpuUsage)) * 100
	}
	return 0
}
