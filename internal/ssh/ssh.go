package ssh

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
)

func RemoteCmd(c *cli.Context) error {
	config, addr, err := getSSHConfig(c)
	if err != nil {
		return err
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("dial: %v", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("session: %v", err)
	}
	defer session.Close()
	var b bytes.Buffer
	session.Stdout = &b

	cmd := "docker ps --format '📦 {{.Names}} | 🔹 {{.Status}} | 🔎 {{.Ports}}'"
	if c.Bool("json") {
		cmd = "docker ps --format '{{json .}}'"
	}
	if err := session.Run(cmd); err != nil {
		return err
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		fmt.Println("No containers")
	} else {
		fmt.Println(out)
	}
	return nil
}

// prosty getSSHConfig: wymaga `--host user@host` i `-i /ścieżka/do/klucza`
func getSSHConfig(c *cli.Context) (*ssh.ClientConfig, string, error) {
	if c.Args().Len() < 1 {
		return nil, "", fmt.Errorf("podaj user@host jako argument")
	}
	userHost := c.Args().Get(0)
	parts := strings.Split(userHost, "@")
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("nieprawidłowy format user@host")
	}
	keyPath := c.String("i")
	if keyPath == "" {
		return nil, "", fmt.Errorf("brak ścieżki do klucza: -i")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, "", err
	}
	config := &ssh.ClientConfig{
		User: parts[0],
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// na szybko ignorujemy weryfikację hosta
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	return config, parts[1] + ":22", nil
}
