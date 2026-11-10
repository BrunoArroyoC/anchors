package functions

import (
	"context"
	"docker-tool/internal/models"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func listImages(ctx context.Context, cli *client.Client) ([]models.DockerModel, error) {
	result, error := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if error != nil {
		return nil, error
	}

	containers := make([]models.DockerModel, 0, len(result.Items))
	for _, value := range result.Items {
		containers = append(containers, toDockerContainers(value))
	}
	return containers, nil
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
