package api

import (
    "bytes"
    "fmt"
    "net/http"

    "github.com/fatih/color"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/urfave/cli/v2"
)

func ServeCmd(c *cli.Context) error {
    port := c.Int("port")
    http.Handle("/metrics", promhttp.Handler())
    http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
        var buf bytes.Buffer
        // you can call local docker ps here and write JSON into buf
        w.Header().Set("Content-Type", "application/json")
        w.Write(buf.Bytes())
    })
    color.Green("Serving on :%d", port)
    return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
