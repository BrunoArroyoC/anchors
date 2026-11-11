// Package bridge turns backend calls into Bubble Tea commands. Each command
// runs one call off the UI goroutine and answers with a message that carries
// either the result or the error.
package bridge

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/functions"
	"docker-tool/internal/models"
)

// callTimeout bounds every Docker call so a hung daemon never freezes a screen.
const callTimeout = 5 * time.Second

// ContainersMsg answers LoadContainers. Err is set when the list failed.
type ContainersMsg struct {
	Containers []models.DockerModel
	Err        error
}

// ImagesMsg answers LoadImages. Err is set when the list failed.
type ImagesMsg struct {
	Images []models.ImageModel
	Err    error
}

// LoadContainers lists every container, running or not.
func LoadContainers(cli *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		cs, err := functions.ListContainers(ctx, cli)
		return ContainersMsg{Containers: cs, Err: err}
	}
}

// LoadImages lists the local images.
func LoadImages(cli *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		is, err := functions.ListImages(ctx, cli)
		return ImagesMsg{Images: is, Err: err}
	}
}
