// Package theme holds the anchors palette: warm paper-white halftone on
// near-black, with international orange (lifebuoys, survival suits) as the
// single accent. Every screen takes its colors from here.
package theme

import "github.com/charmbracelet/lipgloss"

// Palette.
const (
	Night  = "#0b0b0d" // background
	Paper  = "#efe9dd" // halftone highlights, wordmark, primary text
	Ash    = "#26262c" // the faintest halftone dots
	Mist   = "#85828c" // secondary text, labels
	Signal = "#ff4f00" // international orange: cursor, keys, focus, trouble
	Panel  = "#111115" // a box floating over the halftone
	Frame  = "#4a4850" // the box's hairline border
	Raise  = "#1c1b21" // a row under the mouse
	Lift   = "#2a1a13" // the selected row: night warmed by the signal
)

// Look is how a container state is drawn. It is the same on every screen:
// alive is bright, stopped is quiet, and anything that needs a look is
// signal orange.
type Look struct {
	Glyph rune
	Color string
	Word  string
}

// ContainerLook maps a Docker state and health status to its Look.
func ContainerLook(state, health string) Look {
	switch state {
	case "running":
		switch health {
		case "unhealthy":
			return Look{'●', Signal, "unhealthy"}
		case "starting":
			return Look{'◌', Paper, "starting"}
		}
		return Look{'●', Paper, "running"}
	case "restarting":
		return Look{'◌', Signal, "restarting"}
	case "dead":
		return Look{'×', Signal, "dead"}
	case "paused":
		return Look{'◐', Mist, "paused"}
	case "exited":
		return Look{'○', Mist, "exited"}
	case "created":
		return Look{'◌', Mist, "created"}
	case "removing":
		return Look{'◌', Mist, "removing"}
	}
	return Look{'○', Mist, state}
}

// Lip Gloss styles for regular text.
var (
	Text  = lipgloss.NewStyle().Foreground(lipgloss.Color(Paper))
	Quiet = lipgloss.NewStyle().Foreground(lipgloss.Color(Mist))
	Key   = lipgloss.NewStyle().Foreground(lipgloss.Color(Signal)).Bold(true)
	Title = lipgloss.NewStyle().Foreground(lipgloss.Color(Paper)).Bold(true)
)
