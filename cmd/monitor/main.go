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
		Usage: "Monitor Docker (state, service, logs, stats, events, remote, serve)",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "JSON output"},
		},
		Commands: []*cli.Command{
			{Name: "state", Usage: "Show container states", Action: docker.StateCmd},
			{
				Name:  "service",
				Usage: "HTTP health checks",
				Flags: []cli.Flag{
					&cli.DurationFlag{Name: "threshold", Value: 200 * time.Millisecond, Usage: "Response time threshold"},
					&cli.StringFlag{Name: "alert", Usage: "Webhook URL for alerts"},
				},
				Action: docker.ServiceCmd,
			},
			{
				Name:   "logs",
				Usage:  "Tail container logs",
				Action: docker.LogsCmd,
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "tail", Value: 100, Usage: "Lines to show"},
					&cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "Follow logs"},
				},
			},
			{
				Name:   "stats",
				Usage:  "Show container stats",
				Action: docker.StatsCmd,
				Flags: []cli.Flag{
					&cli.DurationFlag{Name: "interval", Value: time.Second, Usage: "Refresh interval"},
				},
			},
			{Name: "events", Usage: "Monitor Docker events", Action: docker.EventsCmd},
			{
				Name:   "remote",
				Usage:  "Monitor remote Docker via SSH",
				Action: ssh.RemoteCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "key", Aliases: []string{"i"}, Usage: "SSH private key path"},
				},
			},
			{
				Name:   "serve",
				Usage:  "Run HTTP & Prometheus server",
				Action: api.ServeCmd,
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "port", Value: 9090, Usage: "Server port"},
				},
			},
		},
		Action: docker.StateCmd,
	}

	if err := app.Run(os.Args); err != nil {
		color.Red("%v", err)
		os.Exit(1)
	}
}
