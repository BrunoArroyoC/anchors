// Package screen holds the Bubble Tea UI. App is the root model: it owns the
// active screen, forwards messages to it, and switches screens.
package screen

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/bridge"
	"docker-tool/internal/screen/dashboard"
	"docker-tool/internal/screen/splash"
)

// App is the root Bubble Tea model.
type App struct {
	docker        *client.Client
	active        tea.Model
	dash          *dashboard.Model
	width, height int
}

// NewApp starts on the splash screen. The dashboard is made up front so it
// can load containers and images while the splash is showing.
func NewApp(docker *client.Client) (*App, error) {
	s, err := splash.New()
	if err != nil {
		return nil, err
	}
	return &App{docker: docker, active: s, dash: dashboard.New(docker)}, nil
}

func (a *App) Init() tea.Cmd { return tea.Batch(a.active.Init(), a.dash.Load()) }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case bridge.ContainersMsg, bridge.ImagesMsg:
		// Data loaded during the splash goes to the dashboard, wherever we are.
		if a.active != tea.Model(a.dash) {
			_, cmd := a.dash.Update(msg)
			return a, cmd
		}
	case splash.DoneMsg:
		a.active = a.dash
		_, cmd := a.dash.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		return a, tea.Batch(a.dash.Init(), cmd)
	}
	var cmd tea.Cmd
	a.active, cmd = a.active.Update(msg)
	return a, cmd
}

func (a *App) View() string { return a.active.View() }
