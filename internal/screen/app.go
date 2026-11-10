// Package screen holds the Bubble Tea UI. App is the root model: it owns the
// active screen, forwards messages to it, and switches screens.
package screen

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/screen/splash"
)

// App is the root Bubble Tea model.
type App struct {
	docker        *client.Client
	active        tea.Model
	width, height int
}

// NewApp starts on the splash screen.
func NewApp(docker *client.Client) (*App, error) {
	s, err := splash.New()
	if err != nil {
		return nil, err
	}
	return &App{docker: docker, active: s}, nil
}

func (a *App) Init() tea.Cmd { return a.active.Init() }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case splash.DoneMsg:
		home := newHome()
		a.active = home
		next, cmd := home.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.active = next
		return a, tea.Batch(home.Init(), cmd)
	}
	var cmd tea.Cmd
	a.active, cmd = a.active.Update(msg)
	return a, cmd
}

func (a *App) View() string { return a.active.View() }
