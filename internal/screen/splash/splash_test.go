package splash

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestFrames plays the intro, a cursor sweep across the sea, a click and the
// exit at several terminal sizes, and checks every frame has the right shape.
//
// Set ANCHORS_SNAPSHOT_DIR to also write each frame as a PNG that draws the
// Braille dots, for looking at the screen without a terminal.
func TestFrames(t *testing.T) {
	dir := os.Getenv("ANCHORS_SNAPSHOT_DIR")
	for _, sz := range [][2]int{{80, 24}, {120, 36}, {200, 55}} {
		m, err := New()
		if err != nil {
			t.Fatal(err)
		}
		cols, rows := sz[0], sz[1]
		m.Update(tea.WindowSizeMsg{Width: cols, Height: rows})
		base := time.Unix(1_000_000, 0)
		at := func(sec float64) { m.Update(frameMsg(base.Add(time.Duration(sec * float64(time.Second))))) }
		run := func(until float64, each func(f float64)) {
			for f := m.now; f < until; f += 1.0 / fps {
				if each != nil {
					each(f)
				}
				at(f)
			}
		}
		check := func(name string) {
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
				writePNG(t, filepath.Join(dir, fmt.Sprintf("%dx%d-%s.png", cols, rows, name)), view, cols, rows)
			}
		}

		at(0)
		run(1.2, nil)
		check("1-writing")
		run(3.4, nil)
		check("2-idle")
		// Sweep the cursor left to right through the sea.
		start := m.now
		run(4.6, func(f float64) {
			p := (f - start) / 1.2
			m.Update(tea.MouseMsg{X: int(float64(cols) * (0.1 + 0.8*p)), Y: rows*3/4 + int(2*p), Action: tea.MouseActionMotion})
		})
		check("3-wake")
		m.Update(tea.MouseMsg{X: cols / 4, Y: rows * 4 / 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		run(4.85, nil)
		check("4-click")
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		run(5.2, nil)
		check("5-leaving")
		run(8, func(float64) {})
		if !m.done {
			t.Fatalf("%dx%d: exit transition never finished", cols, rows)
		}
	}
}

// writePNG redraws the ANSI frame: 8x16 px cells, each Braille dot a 3x3 square.
func writePNG(t *testing.T, path, view string, cols, rows int) {
	const cw, ch = 8, 16
	img := image.NewNRGBA(image.Rect(0, 0, cols*cw, rows*ch))
	bit := [4][2]int{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	for r, line := range strings.Split(view, "\n") {
		var fg, bg color.NRGBA
		x := 0
		for i := 0; i < len(line); {
			if loc := sgr.FindStringIndex(line[i:]); loc != nil && loc[0] == 0 {
				p := strings.Split(strings.TrimSuffix(line[i+2:i+loc[1]], "m"), ";")
				if len(p) == 5 {
					v := func(k int) uint8 { n, _ := strconv.Atoi(p[k]); return uint8(n) }
					c := color.NRGBA{v(2), v(3), v(4), 255}
					if p[0] == "38" {
						fg = c
					} else {
						bg = c
					}
				}
				i += loc[1]
				continue
			}
			ru, size := []rune(line[i:])[0], len(string([]rune(line[i:])[0]))
			i += size
			for yy := 0; yy < ch; yy++ {
				for xx := 0; xx < cw; xx++ {
					img.Set(x*cw+xx, r*ch+yy, bg)
				}
			}
			switch {
			case ru >= 0x2800 && ru <= 0x28ff:
				b := int(ru - 0x2800)
				for dy := 0; dy < 4; dy++ {
					for dx := 0; dx < 2; dx++ {
						if b&bit[dy][dx] != 0 {
							for yy := 0; yy < 3; yy++ {
								for xx := 0; xx < 3; xx++ {
									img.Set(x*cw+1+dx*4+xx, r*ch+1+dy*4+yy, fg)
								}
							}
						}
					}
				}
			case ru != ' ':
				for yy := 5; yy < 12; yy++ {
					for xx := 1; xx < 7; xx++ {
						img.Set(x*cw+xx, r*ch+yy, fg)
					}
				}
			}
			x++
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
