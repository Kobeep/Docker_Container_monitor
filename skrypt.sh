#!/usr/bin/env bash
set -euo pipefail

# Scaffold a fresh Docker_Container_monitor Go CLI project
# Use current directory name as module path
MODULE=$(basename "$PWD")
GO_VERSION=1.22
CLI_VERSION=v2.27.6
COLOR_VERSION=v1.13.0
PROM_VERSION=v1.22.0

# 1. Create go.mod
cat > go.mod <<EOF
module ${MODULE}

go ${GO_VERSION}

require (
    github.com/urfave/cli/v2 ${CLI_VERSION}
    github.com/fatih/color ${COLOR_VERSION}
    github.com/prometheus/client_golang ${PROM_VERSION}
)
EOF

echo "Created go.mod for module '${MODULE}'"

# 2. Create directory structure
mkdir -p cmd/monitor internal/docker internal/ssh internal/api config

# 3. cmd/monitor/main.go
cat > cmd/monitor/main.go <<EOF
package main

import (
    "os"
    "time"

    "github.com/fatih/color"
    "github.com/urfave/cli/v2"
    "${MODULE}/internal/docker"
    "${MODULE}/internal/ssh"
    "${MODULE}/internal/api"
)

func main() {
    app := &cli.App{
        Name:  "monitor",
        Usage: "Monitor Docker: state, service, logs, stats, events, remote, serve",
        Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "JSON output"}},
        Commands: []*cli.Command{
            {Name: "state", Usage: "Show container states", Action: docker.StateCmd},
            {Name: "service", Usage: "HTTP health checks", Flags: []cli.Flag{
                &cli.DurationFlag{Name: "threshold", Value: 200 * time.Millisecond, Usage: "Response time threshold"},
                &cli.StringFlag{Name: "alert", Usage: "Webhook URL for alerts"},
            }, Action: docker.ServiceCmd},
            {Name: "logs", Usage: "Tail container logs", Flags: []cli.Flag{
                &cli.IntFlag{Name: "tail", Value: 100, Usage: "Lines to show"},
                &cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "Follow logs"},
            }, Action: docker.LogsCmd},
            {Name: "stats", Usage: "Show container stats", Flags: []cli.Flag{
                &cli.DurationFlag{Name: "interval", Value: time.Second, Usage: "Refresh interval"},
            }, Action: docker.StatsCmd},
            {Name: "events", Usage: "Monitor Docker events", Action: docker.EventsCmd},
            {Name: "remote", Usage: "Remote Docker via SSH", Flags: []cli.Flag{
                &cli.StringFlag{Name: "key", Aliases: []string{"i"}, Usage: "SSH key path"},
            }, Action: ssh.RemoteCmd},
            {Name: "serve", Usage: "Run HTTP & Prometheus server", Flags: []cli.Flag{
                &cli.IntFlag{Name: "port", Value: 9090, Usage: "Server port"},
            }, Action: api.ServeCmd},
        },
        Action: docker.StateCmd,
    }

    if err := app.Run(os.Args); err != nil {
        color.Red("%v", err)
        os.Exit(1)
    }
}
EOF

echo "Created cmd/monitor/main.go"

# 4. internal/docker/state.go
cat > internal/docker/state.go <<EOF
package docker

import (
    "fmt"
    "os/exec"
    "strings"

    "github.com/fatih/color"
    "github.com/urfave/cli/v2"
)

// StateCmd lists containers and their states
func StateCmd(c *cli.Context) error {
    args := []string{"ps", "--format", "📂 {{.Names}}: 🔹 {{.Status}}"}
    if c.Bool("json") {
        args = []string{"ps", "--format", "{{json .}}"}
    }
    out, err := exec.Command("docker", args...).CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker ps failed: %v\n%s", err, out)
    }
    text := strings.TrimSpace(string(out))
    if text == "" {
        color.Yellow("No containers found")
        return nil
    }
    if c.Bool("json") {
        lines := strings.Split(text, "\n")
        fmt.Println("[" + strings.Join(lines, ",") + "]")
    } else {
        color.Green("Containers:")
        fmt.Println(text)
    }
    return nil
}
EOF

echo "Created internal/docker/state.go"

# 5. internal/docker/logs.go
cat > internal/docker/logs.go <<EOF
package docker

import (
    "context"
    "fmt"
    "os"
    "os/exec"

    "github.com/urfave/cli/v2"
)

// LogsCmd tails or follows logs of a container
func LogsCmd(c *cli.Context) error {
    if c.Args().Len() < 1 {
        return fmt.Errorf("provide container name")
    }
    name := c.Args().Get(0)
    args := []string{"logs", "--tail", fmt.Sprint(c.Int("tail"))}
    if c.Bool("follow") {
        args = append(args, "-f")
    }
    args = append(args, name)

    cmd := exec.CommandContext(c.Context, "docker", args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
EOF

echo "Created internal/docker/logs.go"

# 6. internal/docker/stats.go
cat > internal/docker/stats.go <<EOF
package docker

import (
    "fmt"
    "os/exec"
    "strings"

    "github.com/urfave/cli/v2"
)

// StatsCmd shows container stats once or continuously
func StatsCmd(c *cli.Context) error {
    containers := c.Args().Slice()
    raw := []string{"stats", "--no-stream", "--format", "{{json .}}"}
    pretty := []string{"stats", "--no-stream", "--format", "📊 {{.Name}} | CPU: {{.CPUPerc}} | MEM: {{.MemUsage}}"}
    args := pretty
    if c.Bool("json") {
        args = raw
    }
    args = append(args, containers...)

    out, err := exec.Command("docker", args...).CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker stats failed: %v\n%s", err, out)
    }
    text := strings.TrimSpace(string(out))
    if c.Bool("json") {
        lines := strings.Split(text, "\n")
        fmt.Println("[" + strings.Join(lines, ",") + "]")
    } else {
        fmt.Println(text)
    }
    return nil
}
EOF

echo "Created internal/docker/stats.go"

# 7. internal/docker/service.go
cat > internal/docker/service.go <<EOF
package docker

import (
    "encoding/json"
    "fmt"
    "net/http"
    "os/exec"
    "strings"
    "sync"
    "time"

    "github.com/fatih/color"
    "github.com/urfave/cli/v2"
)

type ServiceCheckResult struct {
    Container string `json:"container"`
    Port      string `json:"port"`
    Status    string `json:"status"`
}

// ServiceCmd performs HTTP health checks on container ports
func ServiceCmd(c *cli.Context) error {
    threshold := c.Duration("threshold")
    webhook := c.String("alert")

    out, err := exec.Command("docker", "ps", "--format", "{{.Names}}: {{.Ports}}").CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker ps failed: %v\n%s", err, out)
    }
    lines := strings.Split(string(out), "\n")
    var wg sync.WaitGroup
    var mu sync.Mutex
    var results []ServiceCheckResult

    for _, line := range lines {
        if strings.TrimSpace(line) == "" {
            continue
        }
        parts := strings.SplitN(line, ": ", 2)
        if len(parts) != 2 {
            continue
        }
        container := parts[0]
        ports := strings.Split(parts[1], ", ")
        for _, p := range ports {
            hostPort := strings.SplitN(p, "->", 2)[0]
            port := strings.Split(hostPort, ":")[1]
            url := fmt.Sprintf("http://localhost:%s", port)
            wg.Add(1)
            go func(cn, prt, url string) {
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
                results = append(results, ServiceCheckResult{cn, prt, status})
                mu.Unlock()
            }(container, port, url)
        }
    }
    wg.Wait()

    if c.Bool("json") {
        b, _ := json.Marshal(results)
        fmt.Println(string(b))
    } else {
        for _, r := range results {
            if r.Status == "available" {
                color.Green("%s on %s OK", r.Container, r.Port)
            } else {
                color.Yellow("%s on %s: %s", r.Container, r.Port, r.Status)
            }
        }
    }
    if webhook != "" {
        for _, r := range results {
            if r.Status != "available" {
                http.Post(webhook, "application/json", strings.NewReader(fmt.Sprintf(`{"container":"%s","port":"%s","status":"%s"}`, r.Container, r.Port, r.Status)))
            }
        }
    }
    return nil
}
EOF

echo "Created internal/docker/service.go"

# 8. internal/docker/events.go
cat > internal/docker/events.go <<EOF
package docker

import (
    "os"
    "os/exec"

    "github.com/urfave/cli/v2"
)

// EventsCmd streams Docker events
func EventsCmd(c *cli.Context) error {
    args := []string{"events"}
    if c.Bool("json") {
        args = []string{"events", "--format", "{{json .}}"}
    }
    cmd := exec.Command("docker", args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
EOF

echo "Created internal/docker/events.go"

# 9. internal/ssh/remote.go
cat > internal/ssh/remote.go <<EOF
package ssh

import (
    "fmt"
    "os/exec"

    "github.com/urfave/cli/v2"
)

// RemoteCmd runs docker ps on a remote host via ssh
func RemoteCmd(c *cli.Context) error {
    if c.Args().Len() < 1 {
        return fmt.Errorf("provide user@host as first argument")
    }
    userhost := c.Args().Get(0)
    sshArgs := []string{}
    if key := c.String("key"); key != "" {
        sshArgs = append(sshArgs, "-i", key)
    }
    sshArgs = append(sshArgs, userhost, "docker", "ps", "--format", "📂 {{.Names}}: 🔹 {{.Status}}")
    if c.Bool("json") {
        // replace last format with JSON
        sshArgs[len(sshArgs)-1] = "{{json .}}"
    }
    cmd := exec.Command("ssh", sshArgs...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
EOF

echo "Created internal/ssh/remote.go"

# 10. internal/api/server.go
cat > internal/api/server.go <<EOF
package api

import (
    "bytes"
    "fmt"
    "net/http"
    "os/exec"
    "strings"

    "github.com/fatih/color"
    "github.com/urfave/cli/v2"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServeCmd starts HTTP server with /metrics and /status
func ServeCmd(c *cli.Context) error {
    port := c.Int("port")
    http.Handle("/metrics", promhttp.Handler())
    http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
        out, err := exec.Command("docker", "ps", "--format", "{{json .}}").Output()
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte("[" + strings.Join(lines, []byte(",")) + "]"))
    })
    color.Green("Starting HTTP server on :%d", port)
    return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
EOF

echo "Created internal/api/server.go"

# 11. config/config.yaml
cat > config/config.yaml <<EOF
hosts:
  - alias: local
    address: "localhost:2375"
thresholds:
  service_response: 200ms
EOF

echo "Created config/config.yaml"

# 12. Tidy module
go mod tidy
echo "Scaffold complete. Build with: go build -o monitor ./cmd/monitor"
