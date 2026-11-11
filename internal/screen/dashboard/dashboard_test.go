package dashboard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/bridge"
	"docker-tool/internal/models"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var base = time.Date(2026, 11, 11, 14, 2, 31, 0, time.Local)

func sampleContainers() []models.DockerModel {
	cs := []models.DockerModel{
		{ID: "3f57d9401f8d1c2a", Name: "web", Image: "nginx:1.27", State: "running", Status: "Up 3 hours (healthy)", Health: "healthy", Ports: "0.0.0.0:8080->80/tcp"},
		{ID: "a81c0d2e9b7f4410", Name: "api", Image: "ghcr.io/acme/api:2.4.1", State: "running", Status: "Up 3 hours (unhealthy)", Health: "unhealthy", Ports: "0.0.0.0:3000->3000/tcp"},
		{ID: "77d0a6c3e1f24b90", Name: "db", Image: "postgres:16", State: "running", Status: "Up 3 hours", Ports: "5432/tcp"},
		{ID: "c4e2b1907d3a5f66", Name: "cache", Image: "redis:7-alpine", State: "paused", Status: "Up 3 hours (Paused)", Ports: "6379/tcp"},
		{ID: "e9a4f0c2b6d81733", Name: "worker", Image: "ghcr.io/acme/worker:2.4.1", State: "exited", Status: "Exited (0) 2 hours ago"},
		{ID: "0b6d2f8e4c1a9e57", Name: "migrate", Image: "ghcr.io/acme/api:2.4.1", State: "exited", Status: "Exited (1) 3 hours ago"},
		{ID: "5d1e7a3c9f0b2c48", Name: "proxy", Image: "traefik:v3.2", State: "restarting", Status: "Restarting (1) 4 seconds ago"},
	}
	for i := range 9 {
		cs = append(cs, models.DockerModel{ID: fmt.Sprintf("%016x", 0xfeed0000+i), Name: fmt.Sprintf("job-%02d", i+1),
			Image: "busybox:1.36", State: "exited", Status: "Exited (0) 2 days ago"})
	}
	return cs
}

func sampleImages() []models.ImageModel {
	day := 24 * time.Hour
	img := func(id string, tags []string, size int64, age time.Duration) models.ImageModel {
		return models.ImageModel{ID: "sha256:" + id, ShortID: id[:12], Tags: tags, Size: size, CreatedAt: base.Add(-age),
			Digests: []string{"nginx@sha256:" + id}}
	}
	return []models.ImageModel{
		img("3b25b682ea82d2db", []string{"nginx:1.27", "nginx:latest"}, 187_400_000, 9*day),
		img("9a1e0c4b77f3a2d5", []string{"postgres:16"}, 432_900_000, 21*day),
		img("1f0e5a2c8d7b6e43", []string{"redis:7-alpine"}, 41_300_000, 40*day),
		img("c0ffee12ab34cd56", []string{"ghcr.io/acme/api:2.4.1"}, 98_200_000, 3*time.Hour),
		img("d00dfeed98765432", []string{"ghcr.io/acme/worker:2.4.1"}, 101_700_000, 3*time.Hour),
		img("7e6d5c4b3a291807", []string{"traefik:v3.2"}, 178_000_000, 70*day),
		img("aa11bb22cc33dd44", []string{"busybox:1.36"}, 4_260_000, 400*day),
		img("0f9e8d7c6b5a4938", nil, 512_000_000, 5*day),
	}
}

// TestScreens plays the dashboard through its states at several sizes and
// checks every frame's shape and that the frame loop stops at rest.
//
// Set ANCHORS_SNAPSHOT_DIR to also write each frame as raw ANSI (.ans).
func TestScreens(t *testing.T) {
	dir := os.Getenv("ANCHORS_SNAPSHOT_DIR")
	for _, sz := range [][2]int{{80, 24}, {120, 36}, {200, 55}} {
		cols, rows := sz[0], sz[1]
		m := New(nil)
		m.Update(tea.WindowSizeMsg{Width: cols, Height: rows})
		m.Init()

		var cmd tea.Cmd
		frame := func(sec float64) {
			_, cmd = m.Update(frameMsg(base.Add(time.Duration(sec * float64(time.Second)))))
		}
		run := func(until float64, each func(f float64)) {
			for f := m.now; f < until; f += 1.0 / fps {
				if each != nil {
					each(f)
				}
				frame(f)
			}
		}
		check := func(name string) {
			t.Helper()
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != rows {
				t.Fatalf("%dx%d %s: %d lines, want %d", cols, rows, name, len(lines), rows)
			}
			for i, l := range lines {
				if w := len([]rune(sgr.ReplaceAllString(l, ""))); w != cols {
					t.Fatalf("%dx%d %s line %d: width %d, want %d", cols, rows, name, i, w, cols)
				}
			}
			if dir != "" {
				p := filepath.Join(dir, fmt.Sprintf("%dx%d-%s.ans", cols, rows, name))
				if err := os.WriteFile(p, []byte(view), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		key := func(s string) {
			var k tea.KeyMsg
			switch s {
			case "tab":
				k = tea.KeyMsg{Type: tea.KeyTab}
			case "esc":
				k = tea.KeyMsg{Type: tea.KeyEsc}
			case "down":
				k = tea.KeyMsg{Type: tea.KeyDown}
			default:
				k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
			}
			m.Update(k)
		}

		frame(0)
		run(0.2, nil)
		check("0-opening")
		run(0.6, nil)
		check("1-loading")

		m.Update(bridge.ContainersMsg{Containers: sampleContainers()})
		m.Update(bridge.ImagesMsg{Images: sampleImages()})
		run(m.now+0.15, nil)
		check("2-printing")
		run(m.now+0.8, nil)
		check("3-loaded")
		run(m.now+6, nil)
		if cmd != nil {
			t.Fatalf("%dx%d: still ticking at rest", cols, rows)
		}

		// Work the list and sweep the cursor through the band.
		key("down")
		key("down")
		start := m.now
		run(start+0.6, func(f float64) {
			p := (f - start) / 0.6
			m.Update(tea.MouseMsg{X: int(float64(cols) * (0.02 + 0.6*p)), Y: int(float64(rows) * (0.95 - 0.3*p)), Action: tea.MouseActionMotion})
		})
		check("4-working")

		// A container changes state on the next refresh.
		cs := sampleContainers()
		cs[4].State, cs[4].Status = "running", "Up 1 second"
		m.Update(bridge.ContainersMsg{Containers: cs})
		run(m.now+0.3, nil)
		check("5-changed")

		key("tab")
		run(m.now+0.12, nil)
		check("6-images-printing")
		run(m.now+0.8, nil)
		key("down")
		run(m.now+0.4, nil)
		check("7-images")

		key("tab")
		run(m.now+0.8, nil)
		check("8-pods")

		key("1")
		key("/")
		for _, r := range "acme" {
			key(string(r))
		}
		run(m.now+0.5, nil)
		check("9-filtering")
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		key("?")
		run(m.now+0.5, nil)
		check("10-keys")
		key("esc")
		key("esc")
		key("/")
		for _, r := range "zzz" {
			key(string(r))
		}
		run(m.now+0.5, nil)
		check("11-no-match")
		key("esc")
		run(m.now+8, nil)
		if cmd != nil {
			t.Fatalf("%dx%d: still ticking after the work settled", cols, rows)
		}

		// Docker down, and an empty machine.
		e := New(nil)
		e.Update(tea.WindowSizeMsg{Width: cols, Height: rows})
		e.Init()
		m = e
		frame(0)
		e.Update(bridge.ContainersMsg{Err: errors.New("list containers: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")})
		e.Update(bridge.ImagesMsg{Err: errors.New("list images: connection refused")})
		run(1, nil)
		check("12-error")
		e.Update(bridge.ContainersMsg{})
		e.Update(bridge.ImagesMsg{})
		run(2, nil)
		check("13-empty")
	}
}
