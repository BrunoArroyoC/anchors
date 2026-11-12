package bridge

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/functions"
	"docker-tool/internal/models"
)

// StartedMsg answers StartContainer. Err is set when Docker refused the start
// (a port already taken, a missing volume, a paused container...).
type StartedMsg struct {
	ID  string
	Err error
}

// StartContainer starts one container. It returns as soon as Docker has
// started it; it does not wait for the container to finish.
func StartContainer(cli *client.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		return StartedMsg{ID: id, Err: functions.StartContainer(ctx, cli, id)}
	}
}

// stopTimeout bounds a stop. Docker answers only once the container is down:
// it sends SIGTERM, waits up to the container's stop timeout (10s unless the
// container sets its own) and then kills it, so callTimeout is too short.
const stopTimeout = 2 * time.Minute

// StoppedMsg answers StopContainer. Err is set when the stop failed.
type StoppedMsg struct {
	ID  string
	Err error
}

// StopContainer stops one container and answers once it is down.
func StopContainer(cli *client.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()
		return StoppedMsg{ID: id, Err: functions.StopContainer(ctx, cli, id)}
	}
}

// removeTimeout bounds a removal: deleting an image's layers from disk can
// take longer than callTimeout.
const removeTimeout = time.Minute

// RestartedMsg answers RestartContainer.
type RestartedMsg struct {
	ID  string
	Err error
}

// RestartContainer stops a container and starts it again. The stop half
// waits like StopContainer, so it gets the same time limit.
func RestartContainer(cli *client.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()
		return RestartedMsg{ID: id, Err: functions.RestartContainer(ctx, cli, id)}
	}
}

// ContainerRemovedMsg answers RemoveContainer.
type ContainerRemovedMsg struct {
	ID  string
	Err error
}

// RemoveContainer removes a stopped container. Docker refuses a running one,
// and keeps its volumes.
func RemoveContainer(cli *client.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
		defer cancel()
		return ContainerRemovedMsg{ID: id, Err: functions.RemoveContainer(ctx, cli, id)}
	}
}

// ImageRemovedMsg answers RemoveImage and RemoveImageTag. Untagged lists the
// tags that are gone, also when a later step failed; Deleted is true when the
// image itself is gone.
type ImageRemovedMsg struct {
	ID       string
	Untagged []string
	Deleted  bool
	Err      error
}

// RemoveImage removes an image and all its tags. Docker refuses to remove an
// image with several tags by its ID, so the tags go one by one and the image
// goes with the last; an untagged image is removed by its ID. When a
// container uses the image, Docker refuses at the last tag, after the
// others are gone: Untagged says which.
func RemoveImage(cli *client.Client, img models.ImageModel) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
		defer cancel()
		if len(img.Tags) == 0 {
			err := functions.RemoveImage(ctx, cli, img.ID)
			return ImageRemovedMsg{ID: img.ID, Deleted: err == nil, Err: err}
		}
		var gone []string
		for _, tag := range img.Tags {
			if err := functions.RemoveImageTag(ctx, cli, tag); err != nil {
				return ImageRemovedMsg{ID: img.ID, Untagged: gone, Err: err}
			}
			gone = append(gone, tag)
		}
		return ImageRemovedMsg{ID: img.ID, Untagged: gone, Deleted: true}
	}
}

// RemoveImageTag removes one name of an image that has others; the image
// stays.
func RemoveImageTag(cli *client.Client, imageID, tag string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
		defer cancel()
		if err := functions.RemoveImageTag(ctx, cli, tag); err != nil {
			return ImageRemovedMsg{ID: imageID, Err: err}
		}
		return ImageRemovedMsg{ID: imageID, Untagged: []string{tag}}
	}
}
