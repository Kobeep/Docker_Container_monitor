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
