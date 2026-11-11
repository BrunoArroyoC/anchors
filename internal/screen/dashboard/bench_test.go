package dashboard

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/bridge"
)

// BenchmarkFrame200x55 measures one animated frame on a large terminal
// with the lighthouse, its beam, the sky, the sea and a full list.
func BenchmarkFrame200x55(b *testing.B) {
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 55})
	m.Init()
	m.Update(bridge.ContainersMsg{Containers: sampleContainers()})
	m.Update(bridge.ImagesMsg{Images: sampleImages()})
	m.Update(frameMsg(base))
	m.Update(tea.MouseMsg{X: 30, Y: 20, Action: tea.MouseActionMotion})
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		m.Update(frameMsg(base.Add(time.Duration(i+1) * time.Second / fps)))
		_ = m.View()
	}
}
