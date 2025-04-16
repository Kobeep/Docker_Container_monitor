package main

import (
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"Docker_Container_monitor/internal/api"
	"Docker_Container_monitor/internal/docker"
	"Docker_Container_monitor/internal/ssh"
)

func main() {
	app := &cli.App{
		Name:  "monitor",
		Usage: "Monitor Docker (logs, stats, services, events, remote, HTTP API)",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "JSON output"},
		},
		Commands: []*cli.Command{
			{Name: "state", Usage: "Container states", Action: docker.StateCmd},
			{Name: "service", Usage: "Service health", Flags: []cli.Flag{
				&cli.DurationFlag{Name: "threshold", Value: 200 * time.Millisecond},
				&cli.StringFlag{Name: "alert"},
			}, Action: docker.ServiceCmd},
			{Name: "logs", Usage: "Tail logs", Flags: []cli.Flag{
				&cli.IntFlag{Name: "tail", Value: 100},
				&cli.BoolFlag{Name: "follow", Aliases: []string{"f"}},
			}, Action: docker.LogsCmd},
			{Name: "stats", Usage: "Live stats", Flags: []cli.Flag{
				&cli.DurationFlag{Name: "interval", Value: time.Second},
			}, Action: docker.StatsCmd},
			{Name: "events", Usage: "Docker events", Action: docker.EventsCmd},
			{Name: "serve", Usage: "Run HTTP API", Flags: []cli.Flag{
				&cli.IntFlag{Name: "port", Value: 9090},
			}, Action: api.ServeCmd},
			{Name: "remote", Usage: "Remote via SSH", Flags: []cli.Flag{
				&cli.StringFlag{Name: "host"},
				&cli.StringFlag{Name: "i"},
			}, Action: ssh.RemoteCmd},
		},
		Action: docker.FullCmd,
	}
	if err := app.Run(os.Args); err != nil {
		color.Red("%v", err)
		os.Exit(1)
	}
}
