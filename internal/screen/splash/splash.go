// Package splash is the anchors title screen, laid out like a modern poster:
// a full-height halftone of a ship's anchor on its chain, and a bold
// condensed wordmark in the dark lower left.
//
// The halftone is the interactive part. The cursor is a lens that swells the
// dots beneath it and leaves an international-orange trail that cools off;
// a click sends a ripple through the screen; Enter washes the print out and
// continues.
package splash

import (
	"fmt"
	"math"
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
	activeFPS = 30
	idleFPS   = 12 // only the background grain moves when nobody touches it
	minCols   = 40
	minRows   = 14

	develop  = 1.4 // seconds for the halftone to develop in
	leaveDur = 0.75
	zoom     = 1.0 // 1 = cover-fit; raise to crop in on the subject
)

var (
	heroAt  = vec{0.69, 0.5} // the anchor's center in the image (0..1)
	focusAt = vec{0.7, 0.5}  // where it should sit on screen (0..1)
)

var (
	cNight  = dots.Hex(theme.Night)
	cPaper  = dots.Hex(theme.Paper)
	cAsh    = dots.Hex(theme.Ash)
	cMist   = dots.Hex(theme.Mist)
	cSignal = dots.Hex(theme.Signal)
)

// Halftone glyphs from smallest to largest.
var ramp = [...]rune{'·', '•', '●'}

type vec struct{ X, Y float64 }

type ripple struct {
	at  vec
	age float64
}

// rect is a block of cells kept free of halftone (behind text).
type rect struct{ c0, r0, c1, r1 int }

func (r rect) has(c, row int) bool { return c >= r.c0 && c < r.c1 && row >= r.r0 && row < r.r1 }

// Model is the splash screen. Create it with New.
type Model struct {
	hero  *dots.Tone
	title *dots.Art

	cv          *dots.Canvas
	imgOff      vec // image placement, in units (1 unit = cell width = half a cell height)
	imgSize     vec
	titleX      int // wordmark position and size, in Braille dots
	titleY      int
	titleW      int
	titleH      int
	titleMask   []bool
	edges       []bool // fine outline of the image, in Braille dots
	keepOut     []rect
	taglineRow  int
	promptRow   int
	heat        []float64 // the cursor trail, per cell
	heatMax     float64
	ripples     []ripple
	start, now  float64
	last        float64
	reduce      bool
	mouseSeen   bool
	mouse       vec // where the cursor is
	lens        vec // where the lens is: it follows the cursor on a spring
	lastMove    float64
	lensOn      float64
	leaving     bool
	leaveT      float64
	done        bool
	cursorLabel string
}

// New loads the art. ANCHORS_REDUCE_MOTION=1 gives a calmer screen: no
// grain, no lens distortion, no ripples, the print already developed.
func New() (*Model, error) {
	hero, err := dots.LoadTone(assets.HeroPNG, 400, 1)
	if err != nil {
		return nil, fmt.Errorf("load hero art: %w", err)
	}
	title, err := dots.LoadArt(assets.TitlePNG)
	if err != nil {
		return nil, fmt.Errorf("load title art: %w", err)
	}
	return &Model{hero: hero, title: title, reduce: os.Getenv("ANCHORS_REDUCE_MOTION") != ""}, nil
}

func (m *Model) tick() tea.Cmd {
	fps := idleFPS
	if m.active() {
		fps = activeFPS
	}
	return tea.Tick(time.Second/time.Duration(fps), func(t time.Time) tea.Msg { return frameMsg(t) })
}

// active is true while anything is moving more than the background grain.
func (m *Model) active() bool {
	return m.now < develop+1 || m.leaving || len(m.ripples) > 0 || m.heatMax > 0.02 ||
		m.now-m.lastMove < 1.5
}

func (m *Model) Init() tea.Cmd { return m.tick() }

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
		p := vec{float64(msg.X) + 0.5, float64(msg.Y)*2 + 1}
		if !m.mouseSeen {
			m.mouse, m.lens, m.mouseSeen = p, p, true
		}
		m.trail(m.mouse, p)
		m.mouse, m.lastMove = p, m.now
		m.cursorLabel = fmt.Sprintf("%03d · %03d", msg.X, msg.Y)
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && !m.reduce {
			m.ripples = append(m.ripples, ripple{at: p})
		}

	case frameMsg:
		m.step(time.Time(msg))
		if m.done {
			return m, func() tea.Msg { return DoneMsg{} }
		}
		return m, m.tick()
	}
	return m, nil
}

func (m *Model) View() string {
	if m.cv == nil {
		return ""
	}
	if m.cv.Cols < minCols || m.cv.Rows < minRows {
		return fmt.Sprintf("ANCHORS\n\nmake the terminal at least %dx%d\nenter continue  q quit", minCols, minRows)
	}
	m.draw()
	return m.cv.Render(cNight)
}

func (m *Model) leave() {
	if m.leaving || m.cv == nil {
		return
	}
	m.leaving, m.leaveT = true, m.now
	if !m.reduce {
		m.ripples = append(m.ripples, ripple{at: vec{float64(m.cv.Cols) * 0.5, float64(m.cv.Rows)}})
	}
}

// resize places the image, the wordmark and the text for the new size.
func (m *Model) resize(cols, rows int) {
	m.cv = dots.NewCanvas(cols, rows)
	m.heat = make([]float64, cols*rows)
	W, H := float64(cols), float64(rows*2)

	aspect := float64(m.hero.W) / float64(m.hero.H)
	// Cover-fit (times zoom), with the anchor's center at focusAt on screen,
	// clamped so the image still covers the whole screen.
	// On narrow terminals the anchor shrinks and moves right to clear the
	// wordmark; outside the image is black.
	scale, focus := zoom, focusAt
	if cols < 110 {
		scale, focus = zoom*0.84, vec{0.76, 0.48}
	}
	iw := math.Max(W, H*aspect) * scale
	ih := iw / aspect
	m.imgSize = vec{iw, ih}
	m.imgOff = vec{W*focus.X - iw*heroAt.X, H*focus.Y - ih*heroAt.Y}

	// Wordmark in the lower left, in Braille dots.
	m.promptRow = rows - 2
	m.taglineRow = m.promptRow - 2
	maxW := float64(cols*2) * 0.44
	maxH := float64(rows*4) * 0.26
	tw := math.Min(maxW, maxH*m.title.Aspect())
	th := tw / m.title.Aspect()
	m.titleW, m.titleH = int(tw), int(th)
	m.titleX = 4 * 2
	m.titleY = (m.taglineRow-1)*4 - m.titleH
	m.titleMask = m.title.Mask(m.titleW, m.titleH, 0.5)

	// Trace the image's edges at Braille resolution (a dot is half a unit).
	// They're drawn in the cells between halftone dots, so the anchor's
	// silhouette stays sharp even where the halftone is coarse.
	cw, ch := m.cv.W, m.cv.H
	m.edges = make([]bool, cw*ch)
	tone := func(x, y float64) float64 {
		return m.toneAt((x/2-m.imgOff.X)/m.imgSize.X, (y/2-m.imgOff.Y)/m.imgSize.Y)
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			gx := tone(fx+1.2, fy) - tone(fx-1.2, fy)
			gy := tone(fx, fy+1.2) - tone(fx, fy-1.2)
			m.edges[y*cw+x] = math.Hypot(gx, gy) > 0.2
		}
	}

	tc0, tr0 := m.titleX/2-1, m.titleY/4-1
	tc1, tr1 := (m.titleX+m.titleW)/2+2, (m.titleY+m.titleH)/4+1
	m.keepOut = []rect{
		{tc0, tr0, tc1, tr1},                     // wordmark
		{2, m.taglineRow, tc1, m.taglineRow + 1}, // tagline
		{2, m.promptRow, cols - 2, m.promptRow + 1},
		{2, 1, cols - 2, 2}, // top labels
	}
}

// trail deposits heat along the cursor's path from a to b.
func (m *Model) trail(a, b vec) {
	if m.cv == nil {
		return
	}
	R := m.reach() * 0.7
	n := int(math.Max(1, math.Hypot(b.X-a.X, b.Y-a.Y)))
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		p := vec{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f}
		for row := max(0, int((p.Y-R)/2)); row <= min(m.cv.Rows-1, int((p.Y+R)/2)); row++ {
			for col := max(0, int(p.X-R)); col <= min(m.cv.Cols-1, int(p.X+R)); col++ {
				d := math.Hypot(float64(col)+0.5-p.X, float64(row)*2+1-p.Y)
				if d < R {
					k := (1 - d/R) * (1 - d/R)
					i := row*m.cv.Cols + col
					m.heat[i] = math.Min(1, m.heat[i]+k*0.35)
				}
			}
		}
	}
}

// reach is the lens radius in units.
func (m *Model) reach() float64 { return math.Max(7, float64(m.cv.Cols)*0.085) }

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

	k := 1 - math.Exp(-dt*10)
	m.lens.X += (m.mouse.X - m.lens.X) * k
	m.lens.Y += (m.mouse.Y - m.lens.Y) * k
	if m.mouseSeen {
		m.lensOn = math.Min(1, m.lensOn+dt*3)
	}

	cool := math.Exp(-dt * 1.4)
	m.heatMax = 0
	for i := range m.heat {
		m.heat[i] *= cool
		m.heatMax = math.Max(m.heatMax, m.heat[i])
	}

	live := m.ripples[:0]
	for _, r := range m.ripples {
		r.age += dt
		if r.age < 1.8 {
			live = append(live, r)
		}
	}
	m.ripples = live

	if m.leaving && m.now-m.leaveT >= leaveDur {
		m.done = true
	}
}

// toneAt samples the hero image, black outside it.
func (m *Model) toneAt(u, v float64) float64 {
	if u < 0 || v < 0 || u > 1 || v > 1 {
		return 0
	}
	return m.hero.At(u, v)
}

func smooth(e0, e1, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-e0)/(e1-e0)))
	return t * t * (3 - 2*t)
}

func hash(x, y int) float64 {
	v := math.Sin(float64(x)*12.9898+float64(y)*78.233) * 43758.5453
	return v - math.Floor(v)
}

// draw paints the halftone, the wordmark and the text.
func (m *Model) draw() {
	cv := m.cv
	cv.Clear()
	t := m.now

	dev := 1.0
	if !m.reduce {
		dev = smooth(0, develop, t)
	}
	fade := 0.0
	if m.leaving {
		fade = math.Min(1, (t-m.leaveT)/leaveDur)
	}
	R := m.reach()

	// The halftone: one dot on every other cell, offset each row, which makes
	// the classic 45° screen with square spacing (cells are twice as tall as wide).
	for row := 0; row < cv.Rows; row++ {
		for col := row % 2; col < cv.Cols; col += 2 {
			skip := false
			for _, k := range m.keepOut {
				if k.has(col, row) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			p := vec{float64(col) + 0.5, float64(row)*2 + 1}

			// Lens: magnify the image under the cursor and swell its dots.
			src := p
			lens := 0.0
			if m.lensOn > 0 {
				d := math.Hypot(p.X-m.lens.X, p.Y-m.lens.Y)
				if d < R {
					lens = (1 - d/R) * (1 - d/R) * m.lensOn
					if !m.reduce {
						src = vec{m.lens.X + (p.X-m.lens.X)*(1-0.4*lens), m.lens.Y + (p.Y-m.lens.Y)*(1-0.4*lens)}
					}
				}
			}

			u := (src.X - m.imgOff.X) / m.imgSize.X
			v := (src.Y - m.imgOff.Y) / m.imgSize.Y
			img := smooth(0.06, 0.8, m.toneAt(u, v)) * dev * (1 - fade)

			grain := 0.0
			if !m.reduce {
				grain = 0.05 + 0.06*(0.5+0.5*math.Sin(t*0.5+p.X*0.045+p.Y*0.07))*(0.5+0.5*math.Sin(t*0.31-p.X*0.021+p.Y*0.05))
			}
			grain *= 1 - fade

			heat := m.heat[row*cv.Cols+col]
			wave := 0.0
			for _, r := range m.ripples {
				d := math.Hypot(p.X-r.at.X, p.Y-r.at.Y)
				radius := r.age * 70
				wave += math.Exp(-math.Pow((d-radius)/3, 2)) * (1 - r.age/1.8)
			}

			val := math.Max(img, grain) + lens*0.22 + heat*0.55 + wave*0.6
			if val < 0.075 {
				continue
			}
			glyph := ramp[0]
			switch {
			case val > 0.62:
				glyph = ramp[2]
			case val > 0.32:
				glyph = ramp[1]
			}
			col0 := dots.Lerp(cAsh, cPaper, float32(smooth(0.05, 0.85, val)))
			accent := math.Min(1, lens*0.8+heat*1.2+wave)
			col0 = dots.Lerp(col0, cSignal, float32(accent*0.9))
			cv.Glyph(col, row, glyph, col0)
		}
	}

	// The outline, only in cells the halftone left empty.
	edge := dots.Lerp(cNight, dots.Lerp(cMist, cPaper, 0.35), float32(dev*(1-fade)))
	for y := 0; y < cv.H; y++ {
		for x := 0; x < cv.W; x++ {
			if m.edges[y*cv.W+x] && (x/2+y/4)%2 == 1 {
				cv.Dot(x, y, edge, 1)
			}
		}
	}

	// The wordmark prints in with a dot dissolve once the image has developed.
	show := 1.0
	if !m.reduce {
		show = smooth(0.5, 1.3, t)
	}
	for y := 0; y < m.titleH; y++ {
		for x := 0; x < m.titleW; x++ {
			if m.titleMask[y*m.titleW+x] && hash(x, y) < show && hash(x+71, y) >= fade {
				cv.Dot(m.titleX+x, m.titleY+y, cPaper, 2)
			}
		}
	}

	if m.leaving {
		return
	}

	// Labels, set small and tracked out like poster furniture.
	in := float32(1)
	if !m.reduce {
		in = float32(smooth(1.2, 1.9, t))
	}
	mist := dots.Lerp(cNight, cMist, in)
	paper := dots.Lerp(cNight, cPaper, in)
	signal := dots.Lerp(cNight, cSignal, in)

	top := "D O C K E R   ·   K U B E R N E T E S"
	if cv.Cols < 60 {
		top = "DOCKER · KUBERNETES"
	}
	cv.Text(4, 1, top, mist)
	if m.cursorLabel != "" && cv.Cols > len(top)+20 {
		cv.Text(cv.Cols-4-len(m.cursorLabel), 1, m.cursorLabel, mist)
	}

	cv.Text(4, m.taglineRow, "your containers and pods, in one harbor", mist)

	col := 4
	for _, part := range []struct {
		s   string
		fg  dots.RGB
		gap int
	}{{"enter", signal, 0}, {" drop anchor", paper, 4}, {"click", signal, 0}, {" ripple", paper, 0}} {
		cv.Text(col, m.promptRow, part.s, part.fg)
		col += len([]rune(part.s)) + part.gap
	}
	cv.Text(cv.Cols-4-len("q quit"), m.promptRow, "q", signal)
	cv.Text(cv.Cols-4-len(" quit"), m.promptRow, " quit", paper)
}
