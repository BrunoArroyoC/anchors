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
	Signal = "#ff4f00" // international orange: cursor, keys, focus
)

// Lip Gloss styles for regular text.
var (
	Text  = lipgloss.NewStyle().Foreground(lipgloss.Color(Paper))
	Quiet = lipgloss.NewStyle().Foreground(lipgloss.Color(Mist))
	Key   = lipgloss.NewStyle().Foreground(lipgloss.Color(Signal)).Bold(true)
	Title = lipgloss.NewStyle().Foreground(lipgloss.Color(Paper)).Bold(true)
)
