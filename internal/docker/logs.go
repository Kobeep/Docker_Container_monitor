package docker

import (
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
