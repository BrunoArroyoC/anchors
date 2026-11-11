// Package dashboard is the main screen of anchors: one manifest of what is
// moored on this machine, in three sections (containers, images, and
// Kubernetes pods, which are not connected yet).
//
// The manifest is a box floating in the middle of a full-screen halftone
// sea. The sea swells while you work, parts under the cursor, leaves an
// orange wake and carries ripples; the box unfolds from its middle when the
// screen opens, rows print in from halftone dots, the selection glides, and
// running containers breathe. All of it settles when you stop touching the
// keyboard and mouse, and the frame loop stops.
package dashboard

import (
	"math"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/client"

	"docker-tool/internal/bridge"
	"docker-tool/internal/models"
	"docker-tool/internal/screen/dots"
	"docker-tool/internal/screen/theme"
)

type frameMsg time.Time
type refreshMsg time.Time

const (
	fps          = 30
	refreshEvery = 5 * time.Second
	calmAfter    = 3.0  // seconds without input before the ambient motion settles
	calmFor      = 1.5  // how long settling takes
	printDur     = 0.22 // one row printing in
	printStagger = 0.018
	flashDur     = 1.6  // a container that changed state glows this long
	boxOpen      = 0.45 // seconds for the box to unfold
	minCols      = 60
	minRows      = 16
)

type section int

const (
	containersTab section = iota
	imagesTab
	podsTab
	numTabs
)

var tabNames = [numTabs]string{"CONTAINERS", "IMAGES", "PODS"}

var (
	cNight  = dots.Hex(theme.Night)
	cPaper  = dots.Hex(theme.Paper)
	cAsh    = dots.Hex(theme.Ash)
	cMist   = dots.Hex(theme.Mist)
	cSignal = dots.Hex(theme.Signal)
	cRaise  = dots.Hex(theme.Raise)
	cPanel  = dots.Hex(theme.Panel)
	cFrame  = dots.Hex(theme.Frame)
	cLift   = dots.Hex(theme.Lift)
)

type vec struct{ X, Y float64 }

type ripple struct {
	at  vec
	age float64
}

// layout is the box and the rows each part of it sits on.
type layout struct {
	bx0, by0, bx1, by1    int // the box, border included
	markX, glyphX, textX  int // selection bar, state glyph, first column
	inR                   int // text stops before this column
	summaryRow, tabRow    int
	ruleRow, headRow      int
	listTop, listRows     int
	detailRule, detailTop int
	detailRows, helpRow   int
}

// inBox reports whether a cell is on the box.
func (l layout) inBox(col, row int) bool {
	return col >= l.bx0 && col <= l.bx1 && row >= l.by0 && row <= l.by1
}

// Model is the dashboard. Create it with New.
type Model struct {
	docker  *client.Client
	host    string
	keys    keyMap
	reduce  bool
	visible bool // false while the splash is still up: data loads, nothing ticks

	cv  *dots.Canvas
	lay layout

	containers []models.DockerModel
	images     []models.ImageModel
	cLoaded    bool
	iLoaded    bool
	cErr, iErr error
	loading    int // loads in flight
	synced     time.Time
	syncDue    bool
	wall       time.Time // wall clock at the latest frame or refresh

	sec       section
	sel       [numTabs]int
	selID     [numTabs]string
	off       [numTabs]int
	filter    string
	filtering bool
	showKeys  bool
	hover     int // list row under the mouse, -1 for none

	// Animation, in seconds since the first frame.
	start, now, last float64
	ticking          bool
	lastInput        float64
	busyUntil        float64
	visPos           float64    // the selection, in rows from the top of the list
	tabX             [2]float64 // the tab underline's left and right edge
	born             map[string]float64
	flash            map[string]float64
	sigs             map[string]string
	images0          map[string]bool
	sectionBorn      float64
	detailBorn       float64
	swellT           float64
	heat             []float64
	heatMax          float64
	ripples          []ripple
	mouse, lens      vec
	mouseIn          bool
	lensOn           float64
}

// New makes the dashboard. docker may be nil (tests feed it messages
// directly). ANCHORS_REDUCE_MOTION=1 gives a calmer screen: no swell, no
// lens distortion, no ripples, rows appear without printing in.
func New(docker *client.Client) *Model {
	m := &Model{
		docker: docker,
		keys:   newKeys(),
		reduce: os.Getenv("ANCHORS_REDUCE_MOTION") != "",
		hover:  -1,
		born:   map[string]float64{}, flash: map[string]float64{},
		sigs: map[string]string{}, images0: map[string]bool{},
	}
	if docker != nil {
		m.host = docker.DaemonHost()
	}
	return m
}

// Load starts listing containers and images, unless a load is in flight.
// The app calls it while the splash is still showing so the data is there
// when the dashboard opens.
func (m *Model) Load() tea.Cmd {
	if m.docker == nil || m.loading > 0 {
		return nil
	}
	m.loading = 2
	return tea.Batch(bridge.LoadContainers(m.docker), bridge.LoadImages(m.docker))
}

func (m *Model) Init() tea.Cmd {
	m.visible = true
	return tea.Batch(m.Load(), refreshTick(), m.kick())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/fps, func(t time.Time) tea.Msg { return frameMsg(t) })
}

func refreshTick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return refreshMsg(t) })
}

// kick starts the frame loop if it isn't running.
func (m *Model) kick() tea.Cmd {
	if m.ticking || !m.visible {
		return nil
	}
	m.ticking = true
	return tick()
}

// busy keeps the frame loop alive for d more seconds.
func (m *Model) busy(d float64) { m.busyUntil = math.Max(m.busyUntil, m.now+d) }

// poke records user input: the ambient motion wakes up.
func (m *Model) poke() {
	m.lastInput = m.now
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)

	case frameMsg:
		m.step(time.Time(msg))
		if m.animating() {
			return m, tick()
		}
		m.ticking = false

	case refreshMsg:
		m.wall = time.Time(msg)
		return m, tea.Batch(m.Load(), refreshTick())

	case bridge.ContainersMsg:
		m.loading = max(0, m.loading-1)
		if msg.Err != nil {
			m.cErr = msg.Err
		} else {
			m.setContainers(msg.Containers)
		}
		return m, m.kick()

	case bridge.ImagesMsg:
		m.loading = max(0, m.loading-1)
		if msg.Err != nil {
			m.iErr = msg.Err
		} else {
			m.setImages(msg.Images)
		}
		return m, m.kick()

	case tea.KeyMsg:
		return m, m.key(msg)

	case tea.MouseMsg:
		return m, m.mouseEvent(msg)
	}
	return m, nil
}

func (m *Model) key(k tea.KeyMsg) tea.Cmd {
	m.poke()
	if m.filtering {
		switch k.Type {
		case tea.KeyEsc:
			m.filtering = false
			m.setFilter("")
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyBackspace:
			if r := []rune(m.filter); len(r) > 0 {
				m.setFilter(string(r[:len(r)-1]))
			} else {
				m.filtering = false
			}
		case tea.KeyUp:
			m.move(-1)
		case tea.KeyDown:
			m.move(1)
		case tea.KeyRunes, tea.KeySpace:
			m.setFilter(m.filter + string(k.Runes))
		}
		return m.kick()
	}

	ks := m.keys
	switch {
	case key.Matches(k, ks.Quit):
		return tea.Quit
	case key.Matches(k, ks.Up):
		m.move(-1)
	case key.Matches(k, ks.Down):
		m.move(1)
	case key.Matches(k, ks.PageUp):
		m.move(-max(1, m.lay.listRows-1))
	case key.Matches(k, ks.PageDown):
		m.move(max(1, m.lay.listRows-1))
	case key.Matches(k, ks.Top):
		m.move(-1 << 30)
	case key.Matches(k, ks.Bottom):
		m.move(1 << 30)
	case key.Matches(k, ks.Next):
		m.switchTo((m.sec + 1) % numTabs)
	case key.Matches(k, ks.Prev):
		m.switchTo((m.sec + numTabs - 1) % numTabs)
	case key.Matches(k, ks.Containers):
		m.switchTo(containersTab)
	case key.Matches(k, ks.Images):
		m.switchTo(imagesTab)
	case key.Matches(k, ks.Pods):
		m.switchTo(podsTab)
	case key.Matches(k, ks.Filter):
		m.filtering = true
	case key.Matches(k, ks.Refresh):
		if m.cv != nil && !m.reduce {
			l := m.lay
			m.ripples = append(m.ripples, ripple{at: vec{float64(l.bx0+l.bx1) / 2, float64(l.by0 + l.by1)}})
		}
		return tea.Batch(m.Load(), m.kick())
	case key.Matches(k, ks.Help):
		m.setKeys(!m.showKeys)
	case key.Matches(k, ks.Back):
		switch {
		case m.showKeys:
			m.setKeys(false)
		case m.filter != "":
			m.setFilter("")
		}
	}
	return m.kick()
}

func (m *Model) mouseEvent(e tea.MouseMsg) tea.Cmd {
	if m.cv == nil {
		return nil
	}
	m.poke()
	l := m.lay
	p := vec{float64(e.X) + 0.5, float64(e.Y)*2 + 1}
	box := l.inBox(e.X, e.Y)
	if !box && !m.mouseIn {
		m.mouse, m.lens = p, p
	}
	if !box {
		m.trail(m.mouse, p)
	}
	m.mouse, m.mouseIn = p, !box
	in := !box

	m.hover = -1
	if vi := e.Y - l.listTop; box && vi >= 0 && vi < l.listRows && m.off[m.sec]+vi < len(m.rows(m.sec)) {
		m.hover = vi
	}

	switch {
	case e.Button == tea.MouseButtonWheelUp:
		m.move(-1)
	case e.Button == tea.MouseButtonWheelDown:
		m.move(1)
	case e.Action == tea.MouseActionPress && e.Button == tea.MouseButtonLeft:
		switch {
		case box && e.Y == l.tabRow:
			for t, s := range m.tabSpans() {
				if e.X >= s[0] && e.X < s[1] {
					m.switchTo(section(t))
				}
			}
		case m.hover >= 0:
			m.move(m.off[m.sec] + m.hover - m.sel[m.sec])
		case in && !m.reduce:
			m.ripples = append(m.ripples, ripple{at: p})
		}
	}
	return m.kick()
}

// move shifts the selection by d rows, clamped, and scrolls to keep it seen.
func (m *Model) move(d int) {
	rows := m.rows(m.sec)
	if len(rows) == 0 {
		return
	}
	s := min(max(m.sel[m.sec]+d, 0), len(rows)-1)
	if s != m.sel[m.sec] {
		m.detailBorn = m.now
	}
	m.sel[m.sec] = s
	m.selID[m.sec] = rows[s].id
	m.scrollIntoView(m.sec)
}

func (m *Model) scrollIntoView(s section) {
	n := m.lay.listRows
	if n <= 0 {
		return
	}
	total := len(m.rows(s))
	off := m.off[s]
	if m.sel[s] < off {
		off = m.sel[s]
	}
	if m.sel[s] >= off+n {
		off = m.sel[s] - n + 1
	}
	m.off[s] = max(0, min(off, total-n))
}

func (m *Model) setKeys(on bool) {
	m.showKeys = on
	if m.cv != nil {
		m.relayout()
	}
}

func (m *Model) switchTo(s section) {
	if s == m.sec {
		return
	}
	m.sec = s
	m.sectionBorn, m.detailBorn = m.now, m.now
	m.busy(1)
	m.hover = -1
}

func (m *Model) setFilter(f string) {
	m.filter = f
	for s := range numTabs {
		m.reanchor(s)
	}
}

// reanchor keeps the selection on the same item after the list changed.
func (m *Model) reanchor(s section) {
	rows := m.rows(s)
	m.sel[s] = min(m.sel[s], max(0, len(rows)-1))
	for i, r := range rows {
		if r.id == m.selID[s] {
			m.sel[s] = i
			break
		}
	}
	if len(rows) > 0 {
		m.selID[s] = rows[m.sel[s]].id
	}
	m.scrollIntoView(s)
}

func (m *Model) setContainers(cs []models.DockerModel) {
	first := !m.cLoaded
	seen := make(map[string]string, len(cs))
	for _, c := range cs {
		sig := c.State + "|" + c.Health
		seen[c.ID] = sig
		old, known := m.sigs[c.ID]
		switch {
		case known && old != sig:
			m.flash[c.ID], m.born[c.ID] = m.now, m.now
			m.busy(flashDur)
		case !known && !first:
			m.born[c.ID] = m.now
			m.busy(1)
		}
	}
	m.sigs = seen
	sortContainers(cs)
	m.containers, m.cLoaded, m.cErr = cs, true, nil
	m.syncDue = true
	if first && m.sec == containersTab {
		m.sectionBorn = m.now
		m.busy(1)
	}
	m.reanchor(containersTab)
}

func (m *Model) setImages(is []models.ImageModel) {
	first := !m.iLoaded
	seen := make(map[string]bool, len(is))
	for _, im := range is {
		seen[im.ID] = true
		if !first && !m.images0[im.ID] {
			m.born[im.ID] = m.now
			m.busy(1)
		}
	}
	m.images0 = seen
	sortImages(is)
	m.images, m.iLoaded, m.iErr = is, true, nil
	m.syncDue = true
	if first && m.sec == imagesTab {
		m.sectionBorn = m.now
		m.busy(1)
	}
	m.reanchor(imagesTab)
}

// resize lays the screen out for a new terminal size.
func (m *Model) resize(cols, rows int) {
	m.cv = dots.NewCanvas(cols, rows)
	m.heat = make([]float64, cols*rows)
	m.relayout()
	m.visPos = float64(m.sel[m.sec] - m.off[m.sec])
	s := m.tabSpans()[m.sec]
	m.tabX = [2]float64{float64(s[0]), float64(s[1])}
}

// relayout places the rows; the panel under the list grows for the keys sheet.
func (m *Model) relayout() {
	W, H := m.cv.Cols, m.cv.Rows
	l := &m.lay
	bw, bh := W-4, H-4
	// The box takes most of the screen, keeping a few cells of sea around
	// it so the background still shows; on very large terminals it stops
	// growing and the sea takes the rest.
	if W > 84 {
		bw = min(W-6, 180)
	}
	if H > 30 {
		bh = min(H-5, 52)
	}
	l.bx0 = (W - bw) / 2
	l.bx1 = l.bx0 + bw - 1
	l.by0 = (H - bh - 2) / 2 // the key hints sit under the box
	l.by1 = l.by0 + bh - 1
	l.helpRow = min(H-1, l.by1+2)
	l.markX, l.glyphX, l.textX = l.bx0+1, l.bx0+3, l.bx0+5
	l.inR = l.bx1 - 1

	y := l.by0 + 1
	l.summaryRow = y
	y++
	if bh >= 26 {
		y++ // room to breathe on tall terminals
	}
	l.tabRow, l.ruleRow, l.headRow, l.listTop = y, y+1, y+2, y+3
	l.detailRows = 4
	if m.showKeys {
		l.detailRows = 6
	}
	l.detailTop = l.by1 - l.detailRows
	l.detailRule = l.detailTop - 1
	l.listRows = max(1, l.detailRule-l.listTop)

	for s := range numTabs {
		m.scrollIntoView(s)
	}
}

// trail warms the band along the cursor's path from a to b.
func (m *Model) trail(a, b vec) {
	R := m.reach() * 0.7
	n := int(math.Max(1, math.Hypot(b.X-a.X, b.Y-a.Y)))
	cols := m.cv.Cols
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		p := vec{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f}
		for row := max(0, int((p.Y-R)/2)); row <= min(m.cv.Rows-1, int((p.Y+R)/2)); row++ {
			for col := max(0, int(p.X-R)); col <= min(cols-1, int(p.X+R)); col++ {
				d := math.Hypot(float64(col)+0.5-p.X, float64(row)*2+1-p.Y)
				if d < R {
					k := (1 - d/R) * (1 - d/R)
					i := row*cols + col
					m.heat[i] = math.Min(1, m.heat[i]+k*0.3)
				}
			}
		}
	}
}

// reach is the lens radius, in units (a cell is 1 wide and 2 tall).
func (m *Model) reach() float64 { return math.Max(6, float64(m.cv.Cols)*0.06) }

// ambient is 1 while someone is working and eases to 0 when they stop.
func (m *Model) ambient() float64 {
	if m.reduce {
		return 0
	}
	return 1 - smooth(calmAfter, calmAfter+calmFor, m.now-m.lastInput)
}

func (m *Model) step(t time.Time) {
	sec := float64(t.UnixNano()) / 1e9
	if m.start == 0 {
		m.start = sec
	}
	m.now = sec - m.start
	dt := math.Max(0, math.Min(m.now-m.last, 0.1))
	m.last = m.now
	m.wall = t
	if m.syncDue {
		m.synced, m.syncDue = t, false
	}
	if m.cv == nil {
		return
	}

	glide := 1 - math.Exp(-dt*18)
	if m.reduce {
		glide = 1
	}
	target := float64(m.sel[m.sec] - m.off[m.sec])
	m.visPos += (target - m.visPos) * glide
	if math.Abs(target-m.visPos) < 0.01 {
		m.visPos = target
	}
	s := m.tabSpans()[m.sec]
	for i, x := range []float64{float64(s[0]), float64(s[1])} {
		m.tabX[i] += (x - m.tabX[i]) * glide
		if math.Abs(x-m.tabX[i]) < 0.05 {
			m.tabX[i] = x
		}
	}

	follow := 1 - math.Exp(-dt*10)
	m.lens.X += (m.mouse.X - m.lens.X) * follow
	m.lens.Y += (m.mouse.Y - m.lens.Y) * follow
	if m.mouseIn {
		m.lensOn = math.Min(1, m.lensOn+dt*3)
	} else {
		m.lensOn = math.Max(0, m.lensOn-dt*3)
	}

	cool := math.Exp(-dt * 1.6)
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

	m.swellT += dt * m.ambient()
}

// animating is true while anything on screen still moves.
func (m *Model) animating() bool {
	if m.cv == nil {
		return false
	}
	s := m.tabSpans()[m.sec]
	return m.loading > 0 || m.now < m.busyUntil || m.ambient() > 0 ||
		m.visPos != float64(m.sel[m.sec]-m.off[m.sec]) ||
		m.tabX != [2]float64{float64(s[0]), float64(s[1])} ||
		m.heatMax > 0.02 || len(m.ripples) > 0 ||
		(m.lensOn > 0 && m.lensOn < 1) ||
		(m.lensOn > 0 && math.Hypot(m.mouse.X-m.lens.X, m.mouse.Y-m.lens.Y) > 0.05)
}

func smooth(e0, e1, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-e0)/(e1-e0)))
	return t * t * (3 - 2*t)
}

func hash(x, y int) float64 {
	v := math.Sin(float64(x)*12.9898+float64(y)*78.233) * 43758.5453
	return v - math.Floor(v)
}
