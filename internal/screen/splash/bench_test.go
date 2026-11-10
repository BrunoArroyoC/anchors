package splash

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func BenchmarkFrame200x55(b *testing.B) {
	m, _ := New()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 55})
	m.Update(tea.MouseMsg{X: 150, Y: 30})
	base := time.Unix(1_000_000, 0)
	size := 0
	for i := 0; b.Loop(); i++ {
		m.Update(frameMsg(base.Add(time.Duration(i) * time.Second / activeFPS)))
		size = len(m.View())
	}
	b.ReportMetric(float64(size), "bytes/frame")
}
