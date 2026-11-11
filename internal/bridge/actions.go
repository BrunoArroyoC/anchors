package bridge

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/functions"
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
