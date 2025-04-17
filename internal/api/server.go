package api

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
)

// ServeCmd starts HTTP server with /metrics & /status
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
		// convert to []string
		str := make([]string, len(lines))
		for i, l := range lines {
			str[i] = string(l)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[" + strings.Join(str, ",") + "]"))
	})
	color.Green("Starting HTTP server on :%d", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
