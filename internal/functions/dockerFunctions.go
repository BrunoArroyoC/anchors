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

// ListContainers returns every container, running or not.
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

// Actions on one container.

// StartContainer starts a stopped container and returns once Docker has
// started it; it doesn't wait for the container to finish. Starting a
// container that is already running is not an error.
func StartContainer(ctx context.Context, cli *client.Client, id string) error {
	if _, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("start container %s: %w", id, err)
	}
	return nil
}

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

// RestartContainer stops a container (like StopContainer, so it can take as
// long as the container's stop timeout) and starts it again.
func RestartContainer(ctx context.Context, cli *client.Client, id string) error {
	_, err := cli.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
	if err != nil {
		return fmt.Errorf("restart container %s: %w", id, err)
	}
	return nil
}

// RemoveContainer removes a stopped container. Docker refuses to remove a
// running one. Its volumes are kept.
func RemoveContainer(ctx context.Context, cli *client.Client, id string) error {
	_, err := cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{})
	if err != nil {
		return fmt.Errorf("remove container %s: %w", id, err)
	}
	return nil
}

// ContainerLogs opens a container's log stream: its last 200 lines, then
// everything it prints until ctx is cancelled, so don't pass a short timeout.
// stdout and stderr come mixed into one stream (split them with
// stdcopy.StdCopy), except for a container started with a TTY, whose stream
// is plain text. The caller must close it.
func ContainerLogs(ctx context.Context, cli *client.Client, id string) (io.ReadCloser, error) {
	out, err := cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true, Tail: "200",
	})
	if err != nil {
		return nil, fmt.Errorf("logs %s: %w", id, err)
	}
	return out, nil
}

// ContainerStats opens a container's live resource usage: a JSON
// container.StatsResponse about every second, until ctx is cancelled. The
// first sample has no previous CPU reading, so CPU usage can only be worked
// out from the second one on. The caller decodes the samples and must close
// the stream.
func ContainerStats(ctx context.Context, cli *client.Client, id string) (io.ReadCloser, error) {
	out, err := cli.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: true})
	if err != nil {
		return nil, fmt.Errorf("container stats %s: %w", id, err)
	}
	return out.Body, nil
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
