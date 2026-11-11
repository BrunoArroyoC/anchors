package screen

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/bridge"
	"docker-tool/internal/models"
	"docker-tool/internal/screen/splash"
)

// TestHandoff checks that data loaded during the splash is on the dashboard
// as soon as the splash is done.
func TestHandoff(t *testing.T) {
	t.Setenv("ANCHORS_REDUCE_MOTION", "1") // rows appear at once instead of printing in
	a, err := NewApp(nil)
	if err != nil {
		t.Fatal(err)
	}
	a.Init()
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.Update(bridge.ContainersMsg{Containers: []models.DockerModel{{ID: "abc", Name: "harbor-web", State: "running", Status: "Up 2 minutes"}}})
	if a.active == tea.Model(a.dash) {
		t.Fatal("dashboard took over before the splash finished")
	}
	_, cmd := a.Update(splash.DoneMsg{})
	if cmd == nil {
		t.Fatal("dashboard didn't start its loop")
	}
	if v := a.View(); !strings.Contains(v, "harbor-web") {
		t.Fatal("container loaded during the splash is missing from the dashboard")
	}
}
