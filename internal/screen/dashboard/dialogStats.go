package dashboard

// The meter is the container dialog's live resource row: CPU as a Braille
// sparkline of the recent samples, memory as a halftone bar. It follows the
// container's stats stream while the container runs; a new sample arrives
// about once a second and redraws the screen without waking the frame loop.

import (
	"fmt"
	"math"

	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/bridge"
	"docker-tool/internal/screen/dots"
)

const cpuHistory = 64 // samples the sparkline remembers

// meter is the dialog's stats state.
type meter struct {
	stream  *bridge.StatsStream
	opening bool // OpenStats sent, no answer yet
	cur     bridge.Stats
	have    bool      // cur holds a sample
	cpu     []float64 // recent CPU percentages, oldest first
}

func (m *Model) openStats() tea.Cmd {
	d := m.dlg
	d.meter.opening = true
	if m.docker == nil {
		return nil
	}
	return bridge.OpenStats(m.docker, d.c.ID)
}

// closeStats stops the stream and forgets the samples: they described a run
// that is over.
func (m *Model) closeStats() {
	d := m.dlg
	if d.meter.stream != nil {
		d.meter.stream.Close()
	}
	d.meter = meter{}
}

// statsMsg handles the stats stream's messages.
func (m *Model) statsMsg(msg tea.Msg) tea.Cmd {
	d := m.dlg
	switch msg := msg.(type) {
	case bridge.StatsOpenedMsg:
		if d == nil || d.c.ID != msg.ID || !d.meter.opening || d.meter.stream != nil {
			if msg.Stream != nil {
				msg.Stream.Close() // nobody is reading it anymore
			}
			return nil
		}
		d.meter.opening = false
		if msg.Err != nil {
			return nil // the row says "no stats"; the dialog works without them
		}
		d.meter.stream = msg.Stream
		return msg.Stream.Next()

	case bridge.StatsMsg:
		if d == nil || msg.Stream != d.meter.stream {
			return nil // a stream that was closed
		}
		if msg.Done {
			d.meter.stream = nil
			return nil
		}
		d.meter.cur, d.meter.have = msg.Stats, true
		if msg.Stats.HasCPU {
			d.meter.cpu = append(d.meter.cpu, msg.Stats.CPUPercent)
			if len(d.meter.cpu) > cpuHistory {
				d.meter.cpu = d.meter.cpu[len(d.meter.cpu)-cpuHistory:]
			}
		}
		return msg.Stream.Next()
	}
	return nil
}

// drawStats is the CPU sparkline and the memory bar, on the row under the
// state.
func (m *Model) drawStats(w dlgLayout, p float64) {
	d := m.dlg
	x, row := w.inL+2, w.statsRow
	mt := d.meter
	label := func(s string) span { return span{s + "  ", cMist} }

	running := d.phase == phaseRunning || d.phase == phaseStopping || d.phase == phaseRestarting
	switch {
	case !running && !mt.have:
		m.write(x, row, w.inR, p, label("CPU"), span{"–", cAsh}, span{"     ", cMist}, label("MEM"), span{"–", cAsh})
		return
	case !mt.have && (mt.opening || mt.stream != nil):
		m.write(x, row, w.inR, p, label("CPU"), span{"measuring…", cAsh})
		return
	case !mt.have:
		m.write(x, row, w.inR, p, label("CPU"), span{"no stats from Docker", cAsh})
		return
	}

	// Narrow dialogs get a shorter sparkline and bar.
	spark, bar := 20, 12
	if w.inR-w.inL < 80 {
		spark, bar = 12, 8
	}

	x = m.write(x, row, w.inR, p, label("CPU"))
	if p >= 1 {
		m.sparkline(x, row, spark)
	}
	x += spark + 1
	cpu := "…"
	if mt.cur.HasCPU {
		cpu = fmt.Sprintf("%.1f%%", mt.cur.CPUPercent)
	}
	x = m.write(x, row, w.inR, p, span{fmt.Sprintf("%-7s", cpu), cPaper}, span{"   ", cMist}, label("MEM"))

	pct := mt.cur.MemPercent
	hot := pct >= 90
	filled := int(math.Round(pct / 100 * float64(bar)))
	if pct > 0 {
		filled = max(1, filled)
	}
	for i := 0; i < bar; i++ {
		g, c := '·', cAsh
		if i < filled {
			g, c = '●', dots.Lerp(cMist, cPaper, 0.75)
			if hot {
				c = cSignal
			}
		}
		m.print(x+i, row, string(g), c, p)
	}
	x += bar + 1
	used := humanSize(int64(mt.cur.MemUsage))
	memC := cPaper
	if hot {
		memC = cSignal
	}
	parts := []span{{used, memC}}
	if mt.cur.MemLimit > 0 {
		parts = append(parts, span{" of " + humanSize(int64(mt.cur.MemLimit)), cMist})
	}
	m.write(x, row, w.inR, p, parts...)
}

// sparkline draws the recent CPU samples in n cells, two samples per cell,
// newest on the right. The scale is one core (100%) or the busiest recent
// sample, whichever is higher, so a busy multi-core container still fits.
func (m *Model) sparkline(x, row, n int) {
	cpu := m.dlg.meter.cpu
	scale := 100.0
	for _, v := range cpu {
		scale = math.Max(scale, v)
	}
	cols := n * 2
	for i := 0; i < cols; i++ {
		dx := x*2 + i
		base := row*4 + 3
		k := len(cpu) - cols + i // the sample in this dot column
		if k < 0 {
			m.cv.Dot(dx, base, cAsh, 1) // a baseline where there is no history yet
			continue
		}
		v := cpu[k]
		h := int(math.Ceil(v / scale * 4))
		if v > 0 {
			h = max(1, h)
		}
		c := dots.Lerp(cMist, cPaper, 0.6)
		if k == len(cpu)-1 {
			c = cPaper // the newest sample
		}
		if h == 0 {
			m.cv.Dot(dx, base, cAsh, 1)
		}
		for j := 0; j < min(h, 4); j++ {
			m.cv.Dot(dx, base-j, c, 2)
		}
	}
}
