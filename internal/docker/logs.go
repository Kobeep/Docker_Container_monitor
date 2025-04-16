package docker

import (
    "context"
    "fmt"
    "io"
    "os"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
    "github.com/urfave/cli/v2"
)

func LogsCmd(c *cli.Context) error {
    cliDocker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return err
    }
    defer cliDocker.Close()
    if c.Args().Len() == 0 {
        return fmt.Errorf("provide container name")
    }
    name := c.Args().Get(0)
    tail := c.Int("tail")
    follow := c.Bool("follow")
    return TailContainerLogs(c.Context, cliDocker, name, tail, follow, os.Stdout)
}

func TailContainerLogs(ctx context.Context, cli *client.Client, container string, tail int, follow bool, out io.Writer) error {
    opts := types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: fmt.Sprint(tail), Follow: follow}
    reader, err := cli.ContainerLogs(ctx, container, opts)
    if err != nil {
        return err
    }
    defer reader.Close()
    _, err = io.Copy(out, reader)
    return err
}
