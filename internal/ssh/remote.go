package ssh

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/urfave/cli/v2"
)

// RemoteCmd runs `ssh [ -i key ] user@host docker ps …`
func RemoteCmd(c *cli.Context) error {
	if c.Args().Len() < 1 {
		return fmt.Errorf("provide user@host")
	}
	target := c.Args().Get(0)
	sshArgs := []string{}
	if key := c.String("key"); key != "" {
		sshArgs = append(sshArgs, "-i", key)
	}
	// default status format
	fmtCmd := "📦 {{.Names}} | 🔹 {{.Status}} | 🔍 {{.Ports}}"
	if c.Bool("json") {
		fmtCmd = "{{json .}}"
	}
	sshArgs = append(sshArgs, target, "docker", "ps", "--format", fmtCmd)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
