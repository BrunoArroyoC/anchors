// Package theme holds the anchors palette: ink on a night sea. Every screen
// takes its colors from here.
package theme

import "github.com/charmbracelet/lipgloss"

// Palette.
const (
	Night    = "#05080c" // background, top of the screen
	Abyss    = "#061a2c" // background, bottom of the screen
	Ink      = "#ece3cf" // the hand-drawn line work: warm paper-white
	Foam     = "#f4f6ff" // brightest highlight, primary text
	Lavender = "#c9c3f0" // secondary text
	Storm    = "#5b5f8f" // quiet text, stars
	SeaDot   = "#2f5888" // resting sea particles
	Glow     = "#7fe0ff" // disturbed particles: bioluminescence
	Lantern  = "#f2a541" // the one warm accent: keys, focus
)

// Lip Gloss styles for regular text.
var (
	Text  = lipgloss.NewStyle().Foreground(lipgloss.Color(Foam))
	Quiet = lipgloss.NewStyle().Foreground(lipgloss.Color(Storm))
	Key   = lipgloss.NewStyle().Foreground(lipgloss.Color(Lantern)).Bold(true)
	Title = lipgloss.NewStyle().Foreground(lipgloss.Color(Ink)).Bold(true)
)
