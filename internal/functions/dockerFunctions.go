package functions

import (
	"context"
	"docker-tool/internal/models"
	"fmt"
	"io"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func ListContainers(ctx context.Context, cli *client.Client) ([]models.DockerModel, error) {
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	containers := make([]models.DockerModel, 0, len(result.Items))
	for _, value := range result.Items {
		containers = append(containers, toDockerContainers(value))
	}
	return containers, nil
}

//Actions for one container

// functions: start and return. No waiting, no printing.
func StartContainer(ctx context.Context, cli *client.Client, id string) error {
	if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("start container %s: %w", id, err)
	}
	return nil
}

// functions: stop and return No waiting, no priting
// StopContainer stops a running container: SIGTERM, then SIGKILL once its
// stop timeout runs out. It returns when the container is down. Stopping a
// container that is already stopped is not an error.
func StopContainer(ctx context.Context, cli *client.Client, id string) error {
	_, err := cli.ContainerStop(ctx, id, client.ContainerStopOptions{})
	if err != nil {
		return fmt.Errorf("stop container %s: %w", id, err)
	}
	return nil
}

// functions: open the log stream. The caller reads it and closes it.
func ContainerLogs(ctx context.Context, cli *client.Client, id string) (io.ReadCloser, error) {
	out, err := cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true, Tail: "200",
	})
	if err != nil {
		return nil, fmt.Errorf("logs %s: %w", id, err)
	}
	return out, nil
}

func toDockerContainers(c container.Summary) models.DockerModel {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	ports := make([]string, 0, len(c.Ports))
	for _, p := range c.Ports {
		ports = append(ports, formatPort(p))
	}

	health := ""
	if c.Health != nil {
		health = string(c.Health.Status)
	}

	return models.DockerModel{
		ID:     c.ID,
		Name:   name,
		Image:  c.Image,
		Ports:  strings.Join(ports, ", "),
		State:  string(c.State),
		Status: c.Status,
		Health: health,
	}

}

func formatPort(p container.PortSummary) string {
	if p.PublicPort != 0 {
		return fmt.Sprintf("%v:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type)
	}
	return fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
}
