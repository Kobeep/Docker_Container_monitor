package tests

import (
    "context"
    "testing"
    "your/module/internal/docker"
)

func TestTailLogsInvalid(t *testing.T) {
    err := docker.TailContainerLogs(context.Background(), nil, "nonexistent", 10, false, nil)
    if err == nil {
        t.Fatal("Expected error for nonexistent container")
    }
}
