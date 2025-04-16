package tests

import (
    "context"
    "testing"
    "your/module/internal/docker"
)

func TestCalcCPUZero(t *testing.T) {
    var stats types.StatsJSON
    if docker.calcCPU(stats) != 0 {
        t.Fatal("Expected 0 CPU for empty stats")
    }
}
