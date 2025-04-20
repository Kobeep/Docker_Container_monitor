package ssh

import (
	"bytes"
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"Docker_Container_monitor/internal/ssh"
)

func RemoteCmd(c *cli.Context) error {
	useJSON := c.Bool("json")
	hostAlias := c.String("host")

	var (
		clientConfig *ssh.ClientConfig
		remoteAddr   string
		err          error
	)

	if hostAlias != "" {
		// from ~/.ssh/config
		clientConfig, remoteAddr, err = getSSHConfig(hostAlias)
		if err != nil {
			return fmt.Errorf("SSH config error for '%s': %v", hostAlias, err)
		}
	} else if c.Args().Len() > 0 {
		// manual user@host + -i key
		userHost := c.Args().Get(0)
		keyPath := c.String("key")
		if keyPath == "" {
			return fmt.Errorf("missing SSH key: use -i <path>")
		}
		clientConfig, remoteAddr, err = getManualSSHConfig(userHost, keyPath)
		if err != nil {
			return fmt.Errorf("SSH manual config error for '%s': %v", userHost, err)
		}
	} else {
		return fmt.Errorf("please specify --host <alias> or <user>@<host> -i <key>")
	}

	if !useJSON {
		color.Cyan("Connecting to %s...", remoteAddr)
	}

	// Dial
	conn, err := ssh.Dial("tcp", remoteAddr, clientConfig)
	if err != nil {
		return fmt.Errorf("failed to dial %s: %v", remoteAddr, err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out

	// Build remote docker command string
	cmdStr := "docker ps --format '📂 {{.Names}}: 🔹 {{.Status}}'"
	if useJSON {
		cmdStr = "docker ps --format '{{json .}}'"
	}

	// Run
	if err := session.Run(cmdStr); err != nil {
		return fmt.Errorf("remote command error: %v\n%s", err, out.String())
	}

	// Print result
	fmt.Print(out.String())
	return nil
}
