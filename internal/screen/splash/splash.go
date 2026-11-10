// Package splash is the anchors title screen: a hand-drawn ship and a
// handwritten wordmark in ink, over a night sea made of particles. The
// drawing stays still; the sea is what moves. The cursor parts the particles
// and leaves a bioluminescent wake, a click sends a ring wave, and Enter
// scatters everything and continues.
package splash

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/screen/assets"
	"docker-tool/internal/screen/dots"
	"docker-tool/internal/screen/theme"
)

// DoneMsg is sent when the exit transition has finished.
type DoneMsg struct{}

type frameMsg time.Time

const (
	fps      = 30
	minCols  = 40
	minRows  = 14
	waterAt  = 0.80 // waterline in the ship drawing, fraction of its height
	writeDur = 1.6  // seconds for the wordmark to write itself in
	leaveDur = 0.8
)

var (
	cNight    = dots.Hex(theme.Night)
	cAbyss    = dots.Hex(theme.Abyss)
	cInk      = dots.Hex(theme.Ink)
	cFoam     = dots.Hex(theme.Foam)
	cLavender = dots.Hex(theme.Lavender)
	cStorm    = dots.Hex(theme.Storm)
	cSeaDot   = dots.Hex(theme.SeaDot)
	cGlow     = dots.Hex(theme.Glow)
	cLantern  = dots.Hex(theme.Lantern)
)

type vec struct{ X, Y float64 }

type particle struct {
	baseX, baseY float64 // rest position before the swell
	x, y, vx, vy float64
	sea          bool
	phase        float64
}

// sprite is artwork sampled to dots and placed on the canvas.
type sprite struct {
	x, y, w, h int
	on         []bool
}

// Model is the splash screen. Create it with New.
type Model struct {
	shipArt, titleArt *dots.Art

	cv         *dots.Canvas
	ship       sprite
	title      sprite
	behindShip []bool // dot cells covered by the ship's hull and sails
	horizon    float64
	subRow     int
	promptRow  int

	parts []particle

	start, now, last float64
	reduce           bool
	rng              *rand.Rand

	mouseSeen bool
	mouse     vec
	mouseVel  vec
	mouseT    float64

	leaving bool
	leaveT  float64
	done    bool
}

// New loads the art. ANCHORS_REDUCE_MOTION=1 gives a calmer screen: no swell,
// the wordmark already written, gentler reactions to the cursor.
func New() (*Model, error) {
	ship, err := dots.LoadArt(assets.ShipPNG)
	if err != nil {
		return nil, fmt.Errorf("load ship art: %w", err)
	}
	title, err := dots.LoadArt(assets.TitlePNG)
	if err != nil {
		return nil, fmt.Errorf("load title art: %w", err)
	}
	return &Model{
		shipArt:  ship,
		titleArt: title,
		reduce:   os.Getenv("ANCHORS_REDUCE_MOTION") != "",
		rng:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 11)),
	}, nil
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg { return frameMsg(t) })
}

func (m *Model) Init() tea.Cmd { return tick() }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "enter", " ":
			m.leave()
		}

	case tea.MouseMsg:
		p := vec{float64(msg.X)*2 + 1, float64(msg.Y)*4 + 2}
		if m.mouseSeen {
			if dt := m.now - m.mouseT; dt > 0.001 {
				v := vec{(p.X - m.mouse.X) / dt, (p.Y - m.mouse.Y) / dt}
				m.mouseVel = vec{m.mouseVel.X*0.5 + v.X*0.5, m.mouseVel.Y*0.5 + v.Y*0.5}
			}
		}
		m.mouse, m.mouseT, m.mouseSeen = p, m.now, true
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.shockwave(p, m.reach()*3, 150)
		}

	case frameMsg:
		m.step(time.Time(msg))
		if m.done {
			return m, func() tea.Msg { return DoneMsg{} }
		}
		return m, tick()
	}
	return m, nil
}

func (m *Model) View() string {
	if m.cv == nil {
		return ""
	}
	if m.cv.Cols < minCols || m.cv.Rows < minRows {
		return fmt.Sprintf("anchors\n\nmake the terminal at least %dx%d\nenter continue  q quit", minCols, minRows)
	}
	return m.cv.Render(m.background, m.draw())
}

// reach is the radius, in dots, within which the cursor moves the sea.
func (m *Model) reach() float64 { return math.Max(9, float64(m.cv.W)*0.055) }

func (m *Model) leave() {
	if m.leaving || m.cv == nil {
		return
	}
	m.leaving, m.leaveT = true, m.now
	m.shockwave(vec{float64(m.cv.W) / 2, m.horizon}, float64(m.cv.W), 220)
}

// shockwave pushes every particle within r away from p.
func (m *Model) shockwave(p vec, r, power float64) {
	if m.reduce {
		power *= 0.4
	}
	for i := range m.parts {
		q := &m.parts[i]
		dx, dy := q.x-p.X, q.y-p.Y
		d := math.Hypot(dx, dy)
		if d < r && d > 0.01 {
			f := (1 - d/r) * power * (0.7 + 0.6*m.rng.Float64())
			q.vx += dx / d * f
			q.vy += dy / d * f
		}
	}
}

// resize lays out the wordmark, the ship and the particle sea.
func (m *Model) resize(cols, rows int) {
	m.cv = dots.NewCanvas(cols, rows)
	W, H := float64(m.cv.W), float64(m.cv.H)

	// Wordmark: about half the width, never more than a fifth of the height.
	tw := math.Min(W*0.5, W-8)
	th := tw / m.titleArt.Aspect()
	if th > H*0.2 {
		th = H * 0.2
		tw = th * m.titleArt.Aspect()
	}
	m.title = place(m.titleArt, int((W-tw)/2), 4, int(tw), int(th), inkThreshold(m.titleArt, tw)+0.05)
	m.subRow = (m.title.y+m.title.h)/4 + 1
	m.promptRow = rows - 2

	// Ship: sits on a waterline two thirds down, so the sea below it is big
	// enough to play with.
	top := float64(m.subRow+2) * 4
	water := H * 0.7
	sh := (water - top) / waterAt
	sw := sh * m.shipArt.Aspect()
	if sw > W*0.6 {
		sw = W * 0.6
		sh = sw / m.shipArt.Aspect()
	}
	m.ship = place(m.shipArt, int((W-sw)/2), int(water-sh*waterAt), int(sw), int(sh), inkThreshold(m.shipArt, sw))
	m.horizon = water

	// Particles dimmed behind the ship: fill each dot row between its first
	// and last ink dot, above the drawn waterline.
	m.behindShip = make([]bool, m.cv.W*m.cv.H)
	for y := 0; y < int(float64(m.ship.h)*waterAt); y++ {
		first, last := -1, -1
		for x := 0; x < m.ship.w; x++ {
			if m.ship.on[y*m.ship.w+x] {
				if first < 0 {
					first = x
				}
				last = x
			}
		}
		for x := first; first >= 0 && x <= last; x++ {
			cx, cy := m.ship.x+x, m.ship.y+y
			if cx >= 0 && cy >= 0 && cx < m.cv.W && cy < m.cv.H {
				m.behindShip[cy*m.cv.W+cx] = true
			}
		}
	}

	// The sea: engraving-style lines of particles, spaced wider with depth.
	m.parts = m.parts[:0]
	for y := m.horizon + 2; y < H; {
		depth := (y - m.horizon) / math.Max(1, H-m.horizon)
		step := 2.6 + depth*2.2
		for x := m.rng.Float64() * 2; x < W; x += step * 0.9 {
			m.parts = append(m.parts, particle{baseX: x, baseY: y, x: x, y: y, sea: true, phase: m.rng.Float64() * math.Pi * 2})
		}
		y += 2.2 + depth*2.8
	}
	// The sky: sparse stars.
	for range int(W * m.horizon / 32) {
		x, y := m.rng.Float64()*W, m.rng.Float64()*(m.horizon-4)
		m.parts = append(m.parts, particle{baseX: x, baseY: y, x: x, y: y, phase: m.rng.Float64() * math.Pi * 2})
	}
}

// inkThreshold asks for more ink per dot the harder the art is shrunk, so
// hatching doesn't clump into solid blobs in small terminals.
func inkThreshold(a *dots.Art, w float64) float64 {
	perDot := float64(a.Bounds.Dx()) / math.Max(1, w)
	return math.Min(0.3, math.Max(0.14, 0.1+0.014*perDot))
}

func place(a *dots.Art, x, y, w, h int, threshold float64) sprite {
	w, h = max(1, w), max(1, h)
	return sprite{x: x, y: y, w: w, h: h, on: a.Mask(w, h, threshold)}
}

// home is where a particle rests at time t: sea lines roll with a slow swell.
func (m *Model) home(p *particle, t float64) (float64, float64) {
	if !p.sea || m.reduce {
		return p.baseX, p.baseY
	}
	depth := (p.baseY - m.horizon) / math.Max(1, float64(m.cv.H)-m.horizon)
	return p.baseX, p.baseY + math.Sin(t*0.7+p.baseX*0.06+p.baseY*0.35)*(0.4+1.4*depth)
}

// step advances the simulation to wall-clock time t.
func (m *Model) step(t time.Time) {
	sec := float64(t.UnixNano()) / 1e9
	if m.start == 0 {
		m.start = sec
	}
	m.now = sec - m.start
	dt := math.Min(m.now-m.last, 0.1)
	m.last = m.now
	if m.cv == nil {
		return
	}

	decay := math.Exp(-dt * 6)
	m.mouseVel = vec{m.mouseVel.X * decay, m.mouseVel.Y * decay}

	R := m.reach()
	push, drag := 1500.0, 7.0
	if m.reduce {
		push, drag = 600, 3
	}
	const sub = 2
	h := dt / sub
	for s := 0; s < sub; s++ {
		for i := range m.parts {
			p := &m.parts[i]
			hx, hy := m.home(p, m.now)
			spring := 16.0
			if m.leaving {
				spring = 0 // let them fly
			}
			ax := (hx-p.x)*spring - p.vx*5.5
			ay := (hy-p.y)*spring - p.vy*5.5
			if m.mouseSeen && !m.leaving {
				dx, dy := p.x-m.mouse.X, p.y-m.mouse.Y
				d := math.Hypot(dx, dy)
				if d < R && d > 0.01 {
					f := (1 - d/R) * (1 - d/R)
					ax += dx / d * f * push
					ay += dy / d * f * push
					ax += m.mouseVel.X * f * drag
					ay += m.mouseVel.Y * f * drag
				}
			}
			p.vx += ax * h
			p.vy += ay * h
			p.x += p.vx * h
			p.y += p.vy * h
		}
	}

	if m.leaving && m.now-m.leaveT >= leaveDur {
		m.done = true
	}
}

func (m *Model) background(row int) dots.RGB {
	k := float32(row) / float32(max(1, m.cv.Rows-1))
	bg := dots.Lerp(cNight, cAbyss, k*k)
	if m.leaving {
		bg = dots.Lerp(bg, cNight, m.leaveProgress())
	}
	return bg
}

func (m *Model) leaveProgress() float32 {
	if !m.leaving {
		return 0
	}
	return float32(math.Min(1, (m.now-m.leaveT)/leaveDur))
}

// hash gives each dot a stable pseudo-random value for dissolves.
func hash(x, y int) float64 {
	v := math.Sin(float64(x)*12.9898+float64(y)*78.233) * 43758.5453
	return v - math.Floor(v)
}

// draw paints the frame into the canvas and returns the text layer.
func (m *Model) draw() *dots.Overlay {
	cv := m.cv
	cv.Clear()
	ov := &dots.Overlay{}
	t := m.now
	leave := float64(m.leaveProgress())

	// Particles: color follows how disturbed they are.
	for i := range m.parts {
		p := &m.parts[i]
		hx, hy := m.home(p, t)
		energy := math.Min(1, math.Hypot(p.vx, p.vy)/45+math.Hypot(p.x-hx, p.y-hy)/9)
		base := cSeaDot
		if !p.sea {
			tw := 0.5 + 0.5*math.Sin(t*1.3+p.phase)
			base = dots.Lerp(dots.Lerp(cNight, cStorm, 0.55), cStorm, float32(tw))
		} else {
			depth := (p.baseY - m.horizon) / math.Max(1, float64(cv.H)-m.horizon)
			base = dots.Lerp(dots.Lerp(cSeaDot, cLavender, 0.18), cSeaDot, float32(math.Min(1, depth*2.5)))
		}
		col := dots.Lerp(base, cGlow, float32(energy))
		if energy > 0.75 {
			col = dots.Lerp(col, cFoam, float32((energy-0.75)/0.25))
		}
		x, y := int(math.Round(p.x)), int(math.Round(p.y))
		if x >= 0 && y >= 0 && x < cv.W && y < cv.H && m.behindShip[y*cv.W+x] {
			if energy < 0.3 {
				continue // the ship hides the calm sea behind it
			}
			col = dots.Lerp(col, cNight, 0.5)
		}
		if leave > 0 {
			col = dots.Lerp(col, cNight, float32(leave*leave))
		}
		cv.Dot(x, y, col, 1)
	}

	// The ship: still ink. It fades in once, and dissolves on leave.
	appear := 1.0
	if !m.reduce {
		appear = math.Min(1, math.Max(0, (t-0.2)/0.9))
	}
	for y := 0; y < m.ship.h; y++ {
		for x := 0; x < m.ship.w; x++ {
			if !m.ship.on[y*m.ship.w+x] {
				continue
			}
			hsh := hash(x, y)
			if hsh > appear || hsh < leave {
				continue
			}
			cv.Dot(m.ship.x+x, m.ship.y+y, cInk, 2)
		}
	}

	// The wordmark writes itself in, left to right, with a glowing nib.
	written := 1.0
	if !m.reduce {
		written = math.Min(1, math.Max(0, (t-0.5)/writeDur))
		written = 1 - math.Pow(1-written, 2)
	}
	nib := written * float64(m.title.w)
	for y := 0; y < m.title.h; y++ {
		for x := 0; x < m.title.w; x++ {
			if !m.title.on[y*m.title.w+x] || float64(x) > nib || hash(x+999, y) < leave {
				continue
			}
			col := cInk
			if written < 1 && nib-float64(x) < 3 {
				col = cGlow
			}
			cv.Dot(m.title.x+x, m.title.y+y, col, 3)
		}
	}

	if m.leaving {
		return ov
	}

	// Subtitle and prompt fade in once the wordmark is written.
	textIn := float32(1)
	if !m.reduce {
		textIn = float32(math.Min(1, math.Max(0, (t-0.5-writeDur)/0.6)))
	}
	if textIn > 0 {
		sub := "docker  ·  kubernetes"
		ov.Put((cv.Cols-len([]rune(sub)))/2, m.subRow, sub, dots.Lerp(cNight, cLavender, textIn))

		type part struct {
			s   string
			col dots.RGB
		}
		key, label, quiet := dots.Lerp(cNight, cLantern, textIn), dots.Lerp(cNight, cLavender, textIn), dots.Lerp(cNight, cStorm, textIn)
		parts := []part{{"enter", key}, {" drop anchor", label}, {"   ·   ", quiet}, {"click", key}, {" make waves", label}, {"   ·   ", quiet}, {"q", key}, {" quit", label}}
		n := 0
		for _, p := range parts {
			n += len([]rune(p.s))
		}
		col := (cv.Cols - n) / 2
		for _, p := range parts {
			ov.Put(col, m.promptRow, p.s, p.col)
			col += len([]rune(p.s))
		}
	}
	return ov
}
