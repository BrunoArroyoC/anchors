package screen

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"docker-tool/internal/screen/theme"
)

// home is a placeholder for where the splash lands until the containers
// screen exists.
type home struct{ width, height int }

func newHome() *home { return &home{} }

func (h *home) Init() tea.Cmd { return nil }

func (h *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "esc" {
			return h, tea.Quit
		}
	}
	return h, nil
}

func (h *home) View() string {
	body := lipgloss.JoinVertical(lipgloss.Center,
		theme.Title.Render("anchored."),
		"",
		theme.Text.Render("Your containers and pods will moor here next."),
		"",
		theme.Key.Render("q")+theme.Quiet.Render(" quit"),
	)
	return lipgloss.Place(h.width, h.height, lipgloss.Center, lipgloss.Center, body,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(theme.Night)))
}
