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

// TestDialog opens a stopped container, starts it and follows its output;
// then a start that Docker refuses; then a click on a running container.
func TestDialog(t *testing.T) {
	dir := os.Getenv("ANCHORS_SNAPSHOT_DIR")
	for _, sz := range [][2]int{{80, 24}, {120, 36}, {200, 55}} {
		cols, rows := sz[0], sz[1]
		m := New(nil)
		m.Update(tea.WindowSizeMsg{Width: cols, Height: rows})
		m.Init()
		var cmd tea.Cmd
		run := func(d float64) {
			for f, end := m.now, m.now+d; f < end; f += 1.0 / fps {
				_, cmd = m.Update(frameMsg(base.Add(time.Duration(f * float64(time.Second)))))
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
				os.WriteFile(filepath.Join(dir, fmt.Sprintf("%dx%d-d%s.ans", cols, rows, name)), []byte(view), 0o644)
			}
		}
		key := func(k tea.KeyMsg) { m.Update(k) }
		runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

		_, cmd = m.Update(frameMsg(base))
		m.Update(bridge.ContainersMsg{Containers: sampleContainers()})
		run(1.5)

		m.selectContainer(sampleContainers()[4].ID) // worker, stopped
		key(tea.KeyMsg{Type: tea.KeyEnter})
		if m.dlg == nil || m.dlg.c.Name != "worker" || m.dlg.phase != phaseReady {
			t.Fatalf("%dx%d: enter on worker should open its dialog ready to start, got %+v", cols, rows, m.dlg)
		}
		run(0.1)
		check("0-unfolding")
		run(0.5)
		check("1-ready")

		key(tea.KeyMsg{Type: tea.KeyEnter})
		if m.dlg.phase != phaseStarting {
			t.Fatalf("%dx%d: enter should start it", cols, rows)
		}
		run(0.3)
		check("2-starting")

		id := m.dlg.c.ID
		m.Update(bridge.StartedMsg{ID: id})
		if m.dlg.phase != phaseRunning || !m.dlg.opening {
			t.Fatalf("%dx%d: after the start it should open the logs", cols, rows)
		}
		s := &bridge.LogStream{ID: id}
		m.Update(bridge.LogsOpenedMsg{ID: id, Stream: s})
		var lines []bridge.LogLine
		lines = append(lines,
			bridge.LogLine{Text: "\x1b[32mINFO\x1b[0m\tworker 2.4.1 starting"},
			bridge.LogLine{Text: "WARN\tqueue \"emails\" is empty, sleeping 5s", Stderr: true},
			bridge.LogLine{Text: "progress 10%\rprogress 100%"},
			bridge.LogLine{Text: "job 1 → " + strings.Repeat("payload ", 40)},
			bridge.LogLine{Text: "日本 emoji 🚢 ok"},
		)
		for i := range 60 {
			lines = append(lines, bridge.LogLine{Text: fmt.Sprintf("INFO\tjob %d done in %dms", i+2, 40+i*7%90)})
		}
		m.Update(bridge.LogsMsg{Stream: s, Lines: lines[:5]})
		run(0.1)
		check("3-printing")
		m.Update(bridge.LogsMsg{Stream: s, Lines: lines[5:]})
		run(0.6)
		check("4-following")

		key(tea.KeyMsg{Type: tea.KeyPgUp})
		key(tea.KeyMsg{Type: tea.KeyPgUp})
		m.Update(bridge.LogsMsg{Stream: s, Lines: lines[:3]}) // more output while scrolled back: the view holds still
		run(0.4)
		check("5-scrolled")
		key(runes("G"))
		if m.dlg.back != 0 {
			t.Fatalf("%dx%d: G should follow again", cols, rows)
		}

		key(tea.KeyMsg{Type: tea.KeyEnter})
		if m.dlg.phase != phaseRunning {
			t.Fatalf("%dx%d: enter must not stop a running container", cols, rows)
		}
		key(runes("x"))
		if m.dlg.phase != phaseStopping {
			t.Fatalf("%dx%d: x should stop it", cols, rows)
		}
		m.Update(bridge.LogsMsg{Stream: s, Lines: []bridge.LogLine{{Text: "SIGTERM received, finishing job 62"}}})
		run(0.3)
		check("5b-stopping")
		m.Update(bridge.StoppedMsg{ID: id})
		if m.dlg.phase != phaseReady || m.dlg.c.State != "exited" {
			t.Fatalf("%dx%d: once stopped the dialog should offer a start", cols, rows)
		}

		m.Update(bridge.LogsMsg{Stream: s, Done: true})
		cs := sampleContainers()
		cs[4].Status = "Exited (0) 1 second ago"
		m.Update(bridge.ContainersMsg{Containers: cs})
		if m.dlg.phase != phaseReady {
			t.Fatalf("%dx%d: once its output ended and it stopped, the dialog should offer a start again", cols, rows)
		}
		run(6)
		check("6-ended")
		if cmd != nil {
			t.Fatalf("%dx%d: still ticking with the dialog at rest", cols, rows)
		}

		// Docker refuses the next start.
		key(runes("s"))
		m.Update(bridge.StartedMsg{ID: id, Err: errors.New("start container " + id + ": Error response from daemon: driver failed programming external connectivity on endpoint worker: Bind for 0.0.0.0:8080 failed: port is already allocated")})
		run(0.6)
		check("7-failed")
		if m.dlg.phase != phaseFailed {
			t.Fatalf("%dx%d: a refused start should fail", cols, rows)
		}

		key(runes("q"))
		if m.dlg != nil {
			t.Fatalf("%dx%d: q should close the dialog, not quit", cols, rows)
		}

		// Clicking a running container opens it and follows its output at once.
		key(runes("g")) // back to the top of the list, where the running ones are
		run(0.5)
		m.Update(tea.MouseMsg{X: m.lay.textX + 2, Y: m.lay.listTop, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		if m.dlg == nil || m.dlg.phase != phaseRunning || !m.dlg.opening {
			t.Fatalf("%dx%d: a click on a running container should open it and its output", cols, rows)
		}
		// A stop that fails leaves it running and says why.
		key(runes("x"))
		m.Update(bridge.StoppedMsg{ID: m.dlg.c.ID, Err: errors.New("stop container " + m.dlg.c.ID + ": context deadline exceeded")})
		if m.dlg.phase != phaseRunning || m.dlg.failMsg == "" {
			t.Fatalf("%dx%d: a failed stop should leave it running and say so", cols, rows)
		}
		run(0.6)
		check("8-stop-failed")

		// A stream that arrives after the dialog closed is closed, not shown.
		key(tea.KeyMsg{Type: tea.KeyEsc})
		m.Update(bridge.LogsOpenedMsg{ID: m.containers[0].ID, Stream: &bridge.LogStream{}})
		if m.dlg != nil {
			t.Fatalf("%dx%d: esc should close the dialog", cols, rows)
		}
	}
}

func TestCleanLog(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[1;31mERR\x1b[0m done":    "ERR done",
		"a\tb":                         "a   b",
		"50%\r100%":                    "100%",
		"\x1b]0;title\x07shown":        "shown",
		"ship 🚢 港":                     "ship □ □",
		"é":                           "e",
		"bell\x07 and nul\x00 removed": "bell and nul removed",
	} {
		if got := cleanLog(in); got != want {
			t.Errorf("cleanLog(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDialogActions plays restart, remove (with its second press) and the
// image dialog: untag, a removal a container blocks, going to that
// container, and removing an image nobody uses.
func TestDialogActions(t *testing.T) {
	dir := os.Getenv("ANCHORS_SNAPSHOT_DIR")
	for _, sz := range [][2]int{{80, 24}, {120, 36}} {
		cols, rows := sz[0], sz[1]
		m := New(nil)
		m.Update(tea.WindowSizeMsg{Width: cols, Height: rows})
		m.Init()
		run := func(d float64) {
			for f, end := m.now, m.now+d; f < end; f += 1.0 / fps {
				m.Update(frameMsg(base.Add(time.Duration(f * float64(time.Second)))))
			}
		}
		snap := func(name string) {
			view := m.View()
			for i, l := range strings.Split(view, "\n") {
				if w := len([]rune(sgr.ReplaceAllString(l, ""))); w != cols {
					t.Fatalf("%dx%d %s line %d: width %d, want %d", cols, rows, name, i, w, cols)
				}
			}
			if dir != "" {
				os.WriteFile(filepath.Join(dir, fmt.Sprintf("%dx%d-a%s.ans", cols, rows, name)), []byte(view), 0o644)
			}
		}
		press := func(s string) {
			k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
			switch s {
			case "enter":
				k = tea.KeyMsg{Type: tea.KeyEnter}
			case "esc":
				k = tea.KeyMsg{Type: tea.KeyEsc}
			}
			m.Update(k)
		}
		fail := func(format string, a ...any) {
			t.Helper()
			t.Fatalf("%dx%d: "+format, append([]any{cols, rows}, a...)...)
		}

		cs := sampleContainers()
		cs[2].Image = "docker.io/library/postgres:16" // a reference anchors can't match to the image: only Docker's refusal tells
		m.Update(frameMsg(base))
		m.Update(bridge.ContainersMsg{Containers: append([]models.DockerModel(nil), cs...)}) // the dashboard sorts what it gets
		m.Update(bridge.ImagesMsg{Images: sampleImages()})
		run(1.5)

		// Restart a running container: the output starts over.
		web := cs[0]
		m.selectContainer(web.ID)
		press("enter")
		m.Update(bridge.LogsOpenedMsg{ID: web.ID, Stream: &bridge.LogStream{ID: web.ID}})
		press("r")
		if m.dlg.phase != phaseRestarting {
			fail("r should restart a running container")
		}
		run(0.3)
		snap("0-restarting")
		m.Update(bridge.RestartedMsg{ID: web.ID})
		if m.dlg.phase != phaseRunning || !m.dlg.opening || m.dlg.c.Status != "just restarted" {
			fail("after a restart the dialog should follow the new run, got %+v", m.dlg)
		}
		if press("d"); m.dlg.armed != actNone {
			fail("a running container can't be removed")
		}
		press("esc")

		// Remove a stopped one: the first press asks, esc takes it back, the second removes.
		worker := cs[4]
		m.selectContainer(worker.ID)
		press("enter")
		press("d")
		if m.dlg.armed != actRemove || m.dlg.phase != phaseReady {
			fail("the first d should ask, not remove")
		}
		run(0.5)
		snap("1-confirm-remove")
		press("esc")
		if m.dlg == nil || m.dlg.armed != actNone {
			fail("esc should take back the remove and keep the dialog")
		}
		press("d")
		press("d")
		if m.dlg.phase != phaseRemoving {
			fail("the second d should remove")
		}
		m.Update(bridge.ContainerRemovedMsg{ID: worker.ID})
		if m.dlg != nil || m.toast != "removed worker" {
			fail("a removed container closes the dialog and says so, got toast %q", m.toast)
		}
		run(0.3)
		snap("2-removed")

		// Images: nginx has two tags and web uses it.
		press("2")
		run(1)
		imgs := sampleImages()
		find := func(tag string) models.ImageModel {
			for _, im := range imgs {
				if len(im.Tags) > 0 && im.Tags[0] == tag || len(im.Tags) == 0 && tag == "" {
					return im
				}
			}
			t.Fatalf("no image %q", tag)
			return models.ImageModel{}
		}
		selectImage := func(id string) {
			for i, r := range m.rows(imagesTab) {
				if r.id == id {
					m.move(i - m.sel[imagesTab])
				}
			}
		}
		nginx := find("nginx:1.27")
		selectImage(nginx.ID)
		press("enter")
		if m.dlg == nil || m.dlg.kind != imageDlg {
			fail("enter on an image should open its dialog")
		}
		run(0.6)
		snap("3-image")

		press("u")
		m.Update(bridge.ImageRemovedMsg{ID: nginx.ID, Untagged: []string{"nginx:1.27"}})
		if len(m.dlg.img.Tags) != 1 || m.dlg.img.Tags[0] != "nginx:latest" || m.dlg.phase != phaseReady {
			fail("untag should drop only the first tag, got %v", m.dlg.img.Tags)
		}
		m.dlg.img.Tags = nginx.Tags // as if the list still had both (the test has no Docker to refresh from)
		press("d")
		if m.dlg.blocker != web.ID || m.dlg.armed != actNone {
			fail("web uses nginx: removing it should point at web instead of asking")
		}
		run(0.5)
		snap("4-blocked")
		press("c")
		if m.dlg == nil || m.dlg.kind != containerDlg || m.dlg.c.ID != web.ID || m.sec != containersTab {
			fail("c should take you to web's dialog")
		}
		press("esc")

		// postgres: anchors can't tell db uses it, Docker can.
		press("2")
		pg := find("postgres:16")
		selectImage(pg.ID)
		press("enter")
		press("d")
		press("d")
		if m.dlg.phase != phaseRemoving {
			fail("an image no container seems to use should be removed after two presses")
		}
		m.Update(bridge.ImageRemovedMsg{ID: pg.ID, Err: errors.New("remove image postgres:16: Error response from daemon: conflict: unable to remove repository reference \"postgres:16\" (must force) - container " + cs[2].ID[:12] + " is using its referenced image " + pg.ShortID)})
		if m.dlg.blocker != cs[2].ID || m.dlg.phase != phaseReady {
			fail("Docker's refusal names db: the dialog should point at it, got blocker %q", m.dlg.blocker)
		}
		run(0.6)
		snap("5-docker-refused")
		press("esc")

		// The dangling image: nobody uses it, it goes by its ID.
		dangling := find("")
		selectImage(dangling.ID)
		press("enter")
		press("d")
		run(0.5)
		snap("6-confirm-image")
		press("d")
		m.Update(bridge.ImageRemovedMsg{ID: dangling.ID, Deleted: true})
		if m.dlg != nil || m.toast != "removed <none>" {
			fail("a removed image closes the dialog, got toast %q", m.toast)
		}
		run(5)
	}
}
