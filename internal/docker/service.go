package docker

import (
    "encoding/json"
    "fmt"
    "net/http"
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

func ServiceCmd(c *cli.Context) error {
    threshold := c.Duration("threshold")
    webhook := c.String("alert")
    return CheckServices(threshold, webhook, c.Bool("json"))
}

func CheckServices(threshold time.Duration, webhook string, useJSON bool) error {
    out, err := exec.Command("docker", "ps", "--format", "{{.Names}}: {{.Ports}}").CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker ps failed: %v %s", err, out)
    }
    lines := strings.Split(string(out), "\n")
    var wg sync.WaitGroup
    var mu sync.Mutex
    var results []ServiceCheckResult
    for _, line := range lines {
        parts := strings.Split(line, ": ")
        if len(parts) != 2 {
            continue
        }
        cName, ports := parts[0], strings.Split(parts[1], ", ")
        for _, p := range ports {
            prt := strings.Split(p, ":")[1]
            url := fmt.Sprintf("http://localhost:%s", prt)
            wg.Add(1)
            go func(cn, pr, url string) {
                defer wg.Done()
                start := time.Now()
                resp, err := http.Get(url)
                status := "unreachable"
                if err == nil {
                    if resp.StatusCode == 200 && time.Since(start) < threshold {
                        status = "available"
                    } else {
                        status = fmt.Sprintf("%s (%v)", resp.Status, time.Since(start))
                    }
                    resp.Body.Close()
                }
                mu.Lock()
                results = append(results, ServiceCheckResult{cn, pr, status})
                mu.Unlock()
            }(cName, prt, url)
        }
    }
    wg.Wait()
    if useJSON {
        b, _ := json.Marshal(results)
        fmt.Println(string(b))
    } else {
        for _, r := range results {
            switch r.Status {
            case "available":
                color.Green("%s on %s OK", r.Container, r.Port)
            default:
                color.Yellow("%s on %s: %s", r.Container, r.Port, r.Status)
            }
        }
    }
    if webhook != "" {
        for _, r := range results {
            if r.Status != "available" {
                http.Post(webhook, "application/json",
                    strings.NewReader(fmt.Sprintf("{\"container\":\"%s\",\"port\":\"%s\",\"status\":\"%s\"}",
                        r.Container, r.Port, r.Status)))
            }
        }
    }
    return nil
}
