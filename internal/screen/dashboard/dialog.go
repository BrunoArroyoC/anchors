package dashboard

// The container dialog opens from a container row (click it, or enter). For
// a stopped container it offers to start it; once the container runs, a
// small terminal under the header follows what it prints, and the dialog
// offers to stop it. A running container's output is followed straight away.

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"docker-tool/internal/bridge"
	"docker-tool/internal/models"
	"docker-tool/internal/screen/dots"
	"docker-tool/internal/screen/theme"
)

type phase int

const (
	phaseReady    phase = iota // not running: the dialog offers to start it
	phaseStarting              // start sent, Docker hasn't answered yet
	phaseRunning               // running: its output is followed, the dialog offers to stop it
	phaseFailed                // Docker refused the start; the terminal says why
	phaseStopping              // stop sent; Docker answers once the container is down
)

const (
	dialogOpen  = 0.22 // seconds for the dialog to unfold
	lineIn      = 0.25 // a new log line printing in
	maxLogLines = 2000 // older lines are dropped
	maxLogRunes = 2000 // a longer line is cut
)

type lineKind int

const (
	stdoutLine lineKind = iota
	stderrLine
	noteLine // something went wrong (orange)
	infoLine // anchors talking, not the container ("output ended")
)

type logLine struct {
	text string
	kind lineKind
	at   float64 // when it arrived, for printing in
}

// dialog is the open container dialog.
type dialog struct {
	c       models.DockerModel // refreshed with the list
	phase   phase
	born    float64
	phaseAt float64
	stream  *bridge.LogStream
	opening bool // OpenLogs sent, no answer yet
	ended   bool // the output stream ended
	logs    []logLine
	back    int  // rows scrolled up from the newest; 0 follows the output
	hover   int  // 1: the action button is under the mouse, 2: close
	stopErr bool // the last stop failed; the terminal says why
}

func (d *dialog) canStart() bool { return d.phase == phaseReady || d.phase == phaseFailed }
func (d *dialog) canStop() bool  { return d.phase == phaseRunning }

// busyPhase is true while Docker is working on a start or a stop.
func (d *dialog) busyPhase() bool { return d.phase == phaseStarting || d.phase == phaseStopping }

func isUp(c models.DockerModel) bool { return c.State == "running" || c.State == "restarting" }

// dlgLayout is where the dialog and its parts sit. It is centred on the
// logbook, so the lighthouse beam still finds the selected row's edge.
type dlgLayout struct {
	x0, y0, x1, y1    int
	inL, inR          int // text runs from inL to before inR
	headRow, stateRow int
	actRow, ruleRow   int // actRow: the action button and what is happening
	termTop, termRows int
	termL, termR      int    // log text runs from termL to before termR
	act, close        [3]int // a button's from, to (exclusive) and row
}

const (
	startLabel = " ▶  START "
	retryLabel = " ↻  RETRY "
	stopLabel  = " ■  STOP  " // all three action labels are the same width
	closeLabel = " esc ✕ "
)

func (m *Model) dialogLayout() dlgLayout {
	l := m.lay
	var w dlgLayout
	bw, bh := l.bx1-l.bx0+1, l.by1-l.by0+1
	dw, dh := min(bw-4, 104), min(bh-2, 32)
	w.x0, w.y0 = l.bx0+(bw-dw)/2, l.by0+(bh-dh)/2
	w.x1, w.y1 = w.x0+dw-1, w.y0+dh-1
	w.inL, w.inR = w.x0+3, w.x1-2
	gap := 1
	if dh < 18 {
		gap = 0
	}
	w.headRow = w.y0 + 1 + gap
	w.stateRow = w.headRow + 1
	w.actRow = w.stateRow + 1 + gap
	w.ruleRow = w.actRow + 1 + gap
	w.termTop = w.ruleRow + 1
	w.termRows = max(1, w.y1-gap-w.termTop)
	w.termL, w.termR = w.x0+5, w.x1-3
	w.act = [3]int{w.inL, w.inL + len([]rune(startLabel)), w.actRow}
	n := len([]rune(closeLabel))
	w.close = [3]int{w.x1 - 2 - n, w.x1 - 2, w.y0}
	return w
}

// openDialog opens the dialog for the selected container.
func (m *Model) openDialog() tea.Cmd {
	r, ok := m.selected()
	if !ok || r.c == nil {
		return nil
	}
	m.closeDialog()
	d := &dialog{c: *r.c, born: m.now, phaseAt: m.now}
	m.dlg, m.hover = d, -1
	m.busy(dialogOpen + 0.4)
	if isUp(d.c) {
		d.phase = phaseRunning
		return m.openLogs()
	}
	return nil
}

func (m *Model) closeDialog() {
	if m.dlg != nil && m.dlg.stream != nil {
		m.dlg.stream.Close()
	}
	m.dlg = nil
}

func (m *Model) openLogs() tea.Cmd {
	d := m.dlg
	d.opening = true
	if m.docker == nil {
		return nil
	}
	return bridge.OpenLogs(m.docker, d.c.ID)
}

// startContainer asks Docker to start the dialog's container. The terminal
// starts empty: the stream that opens next brings the recent output.
func (m *Model) startContainer() tea.Cmd {
	d := m.dlg
	if d == nil || !d.canStart() {
		return nil
	}
	if d.stream != nil {
		d.stream.Close()
	}
	*d = dialog{c: d.c, born: d.born, phase: phaseStarting, phaseAt: m.now}
	if m.docker == nil {
		return nil
	}
	return bridge.StartContainer(m.docker, d.c.ID)
}

// stopContainer asks Docker to stop the dialog's container. The output stays
// open meanwhile, so whatever it prints while shutting down shows up.
func (m *Model) stopContainer() tea.Cmd {
	d := m.dlg
	if d == nil || !d.canStop() {
		return nil
	}
	d.phase, d.phaseAt, d.stopErr = phaseStopping, m.now, false
	if m.docker == nil {
		return nil
	}
	return bridge.StopContainer(m.docker, d.c.ID)
}

// pressButton does what the action button says.
func (m *Model) pressButton() tea.Cmd {
	if m.dlg.canStop() {
		return m.stopContainer()
	}
	return m.startContainer()
}

// dialogMsg handles the bridge messages that belong to the dialog.
func (m *Model) dialogMsg(msg tea.Msg) tea.Cmd {
	d := m.dlg
	switch msg := msg.(type) {
	case bridge.StartedMsg:
		if d == nil || d.c.ID != msg.ID || d.phase != phaseStarting {
			return m.Load() // the dialog was closed meanwhile; the list still wants the news
		}
		d.phaseAt = m.now
		if msg.Err != nil {
			d.phase = phaseFailed
			m.addLine(noteLine, strings.ReplaceAll(msg.Err.Error(), d.c.ID, shortID(d.c.ID)))
			return m.kick()
		}
		d.phase = phaseRunning
		d.c.State, d.c.Status = "running", "just started" // until the list refreshes
		m.busy(flashDur)
		return tea.Batch(m.openLogs(), m.Load(), m.kick())

	case bridge.StoppedMsg:
		if d == nil || d.c.ID != msg.ID || d.phase != phaseStopping {
			return m.Load()
		}
		d.phaseAt = m.now
		if msg.Err != nil {
			d.phase, d.stopErr = phaseRunning, true
			m.addLine(noteLine, strings.ReplaceAll(msg.Err.Error(), d.c.ID, shortID(d.c.ID)))
			return m.kick()
		}
		d.phase = phaseReady
		d.c.State, d.c.Status = "exited", "just stopped" // until the list refreshes
		m.busy(flashDur)
		return tea.Batch(m.Load(), m.kick())

	case bridge.LogsOpenedMsg:
		if d == nil || d.c.ID != msg.ID || !d.opening || d.stream != nil {
			if msg.Stream != nil {
				msg.Stream.Close() // nobody is reading it anymore
			}
			return nil
		}
		d.opening = false
		if msg.Err != nil {
			d.ended = true
			m.addLine(noteLine, msg.Err.Error())
			return m.kick()
		}
		d.stream = msg.Stream
		return msg.Stream.Next()

	case bridge.LogsMsg:
		if d == nil || msg.Stream != d.stream {
			return nil // a stream that was closed
		}
		for _, l := range msg.Lines {
			k := stdoutLine
			if l.Stderr {
				k = stderrLine
			}
			m.addLine(k, l.Text)
		}
		if !msg.Done {
			return tea.Batch(msg.Stream.Next(), m.kick())
		}
		d.stream, d.ended = nil, true
		if msg.Err != nil {
			m.addLine(noteLine, msg.Err.Error())
		} else {
			m.addLine(infoLine, "output ended")
		}
		return tea.Batch(m.Load(), m.kick()) // it probably stopped: refresh its state
	}
	return nil
}

// addLine appends one line to the terminal. Scrolled back, the view stays
// on the same lines instead of following.
func (m *Model) addLine(kind lineKind, text string) {
	d := m.dlg
	text = cleanLog(text)
	if d.back > 0 {
		w := m.dialogLayout()
		d.back += len(wrap(text, w.termR-w.termL))
	}
	d.logs = append(d.logs, logLine{text: text, kind: kind, at: m.now})
	if len(d.logs) > maxLogLines*5/4 {
		d.logs = append([]logLine(nil), d.logs[len(d.logs)-maxLogLines:]...)
	}
	m.busy(lineIn + 0.05)
}

// dialogSynced updates the dialog's container from a fresh list. When its
// output has ended and it is no longer up, the dialog offers to start it.
func (m *Model) dialogSynced() {
	d := m.dlg
	if d == nil {
		return
	}
	for _, c := range m.containers {
		if c.ID == d.c.ID {
			d.c = c
			break
		}
	}
	if d.phase == phaseRunning && d.ended && !isUp(d.c) {
		d.phase, d.phaseAt = phaseReady, m.now
	}
}

func (m *Model) dialogKey(k tea.KeyMsg) tea.Cmd {
	d, ks := m.dlg, m.dkeys
	w := m.dialogLayout()
	switch {
	case key.Matches(k, ks.Close):
		m.closeDialog()
	case key.Matches(k, ks.Start):
		return tea.Batch(m.startContainer(), m.kick())
	case key.Matches(k, ks.Stop):
		return tea.Batch(m.stopContainer(), m.kick())
	case key.Matches(k, ks.Up):
		m.scrollLogs(1)
	case key.Matches(k, ks.Down):
		m.scrollLogs(-1)
	case key.Matches(k, ks.PageUp):
		m.scrollLogs(max(1, w.termRows-1))
	case key.Matches(k, ks.PageDown):
		m.scrollLogs(-max(1, w.termRows-1))
	case key.Matches(k, ks.Top):
		m.scrollLogs(1 << 30)
	case key.Matches(k, ks.Follow):
		d.back = 0
	}
	return m.kick()
}

func (m *Model) dialogMouse(e tea.MouseMsg) tea.Cmd {
	d := m.dlg
	w := m.dialogLayout()
	on := func(b [3]int) bool { return b[1] > 0 && e.Y == b[2] && e.X >= b[0] && e.X < b[1] }
	d.hover = 0
	switch {
	case on(w.act) && !d.busyPhase():
		d.hover = 1
	case on(w.close):
		d.hover = 2
	}
	switch {
	case e.Button == tea.MouseButtonWheelUp:
		m.scrollLogs(3)
	case e.Button == tea.MouseButtonWheelDown:
		m.scrollLogs(-3)
	case e.Action == tea.MouseActionPress && e.Button == tea.MouseButtonLeft:
		switch d.hover {
		case 1:
			return tea.Batch(m.pressButton(), m.kick())
		case 2:
			m.closeDialog()
		}
	}
	return m.kick()
}

// scrollLogs moves the terminal n rows back in time (negative: forward).
func (m *Model) scrollLogs(n int) {
	d := m.dlg
	w := m.dialogLayout()
	d.back = max(0, d.back+n)
	_, d.back = d.tail(w.termR-w.termL, w.termRows)
}

// vrow is one terminal row: a log line, or a piece of a wrapped one.
type vrow struct {
	text string
	kind lineKind
	at   float64
	cont bool // continues the line above
}

// tail is the n rows on screen, oldest first, w cells wide, and the scroll
// position clamped to what there is.
func (d *dialog) tail(w, n int) ([]vrow, int) {
	need := d.back + n
	var rev []vrow
	for i := len(d.logs) - 1; i >= 0 && len(rev) < need; i-- {
		l := d.logs[i]
		parts := wrap(l.text, w)
		for j := len(parts) - 1; j >= 0; j-- {
			rev = append(rev, vrow{parts[j], l.kind, l.at, j > 0})
		}
	}
	back := min(d.back, max(0, len(rev)-n))
	vis := rev[back:min(len(rev), back+n)]
	out := make([]vrow, len(vis))
	for i, r := range vis {
		out[len(vis)-1-i] = r
	}
	return out, back
}

// wrap cuts s into pieces of at most w cells, at a space when there is one
// in the second half of the piece.
func wrap(s string, w int) []string {
	r := []rune(s)
	if w <= 0 || len(r) <= w {
		return []string{s}
	}
	var out []string
	for len(r) > w {
		cut := w
		for i := w; i > w/2; i-- {
			if r[i] == ' ' {
				cut = i
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
		if r[0] == ' ' {
			r = r[1:]
		}
	}
	return append(out, string(r))
}

var escapes = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// cleanLog makes a log line safe to draw one rune per cell: no colour codes
// or other escapes, tabs expanded, control and zero-width runes dropped, and
// double-width runes (CJK, emoji) replaced so the row keeps its width.
func cleanLog(s string) string {
	s = escapes.ReplaceAllString(s, "")
	if i := strings.LastIndexByte(s, '\r'); i >= 0 && i < len(s)-1 {
		s = s[i+1:] // a progress bar redraws itself with \r: keep what it shows last
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxLogRunes {
			break
		}
		switch {
		case r == '\t':
			for k := 4 - n%4; k > 0; k-- {
				b.WriteByte(' ')
				n++
			}
			continue
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
			continue
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
			continue
		case wide(r):
			r = '□'
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// wide reports runes that take two terminal cells.
func wide(r rune) bool {
	switch {
	case r < 0x1100:
		return false
	case r <= 0x115f, r >= 0x2e80 && r <= 0xa4cf, r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff, r >= 0xfe30 && r <= 0xfe4f, r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6, r >= 0x1f300 && r <= 0x1faff, r >= 0x20000 && r <= 0x3fffd:
		return true
	}
	return false
}

// drawDialog dims the screen behind it (all but the key hints) and draws
// the dialog on top.
func (m *Model) drawDialog(hints [2]int) {
	cv, d := m.cv, m.dlg
	w := m.dialogLayout()
	open := 1.0
	if !m.reduce {
		open = smooth(0, dialogOpen, m.now-d.born)
	}

	dim := float32(0.62 * open)
	for row := 0; row < cv.Rows; row++ {
		for col := 0; col < cv.Cols; col++ {
			if row == m.lay.helpRow && col >= hints[0] && col < hints[1] {
				continue // the dialog's key hints
			}
			cv.Fade(col, row, cNight, dim)
		}
	}

	// Unfold from the middle, like the logbook.
	h := w.y1 - w.y0 + 1
	rows := max(2, int(math.Round(float64(h)*open)))
	top := w.y0 + (h-rows)/2
	bot := top + rows - 1
	shadow := dots.Lerp(cNight, dots.RGB{}, 0.6)
	for row := top; row <= bot+1; row++ {
		for col := w.x0; col <= w.x1+1; col++ {
			cv.ClearCell(col, row)
			switch {
			case row <= bot && col <= w.x1:
				cv.Back(col, row, cPanel)
			case row > top && col > w.x0:
				cv.Back(col, row, shadow)
			}
		}
	}
	edge := dots.Lerp(cFrame, cMist, 0.35)
	for col := w.x0 + 1; col < w.x1; col++ {
		cv.Glyph(col, top, '─', edge)
		cv.Glyph(col, bot, '─', edge)
	}
	for row := top + 1; row < bot; row++ {
		cv.Glyph(w.x0, row, '│', edge)
		cv.Glyph(w.x1, row, '│', edge)
	}
	cv.Glyph(w.x0, top, '╭', edge)
	cv.Glyph(w.x1, top, '╮', edge)
	cv.Glyph(w.x0, bot, '╰', edge)
	cv.Glyph(w.x1, bot, '╯', edge)
	if open < 1 {
		return
	}

	p := 1.0
	if !m.reduce {
		p = (m.now - d.born - dialogOpen) / 0.18
	}
	m.write(w.x0+2, w.y0, w.close[0]-1, 1, span{" C O N T A I N E R ", cPaper})
	cc := cMist
	if d.hover == 2 {
		cc = cPaper
		for col := w.close[0]; col < w.close[1]; col++ {
			cv.Back(col, w.y0, cLift)
		}
	}
	m.write(w.close[0], w.y0, w.close[1], 1, span{closeLabel, cc})

	m.drawDialogHead(w, p)
	m.drawDialogAction(w, p)

	m.write(w.inL, w.ruleRow, w.inR, p, span{"OUTPUT", cMist})
	for col := w.inL + 8; col < w.inR; col += 2 {
		cv.Glyph(col, w.ruleRow, '·', cFrame)
	}
	m.drawTerminal(w)
}

// drawDialogHead is the container's name, image and state.
func (m *Model) drawDialogHead(w dlgLayout, p float64) {
	d := m.dlg
	c := d.c
	look := theme.ContainerLook(c.State, c.Health)
	g, gc := look.Glyph, dots.Hex(look.Color)
	if look.Word == "running" {
		amb := m.ambient()
		gc = dots.Lerp(cMist, cPaper, float32(1-amb*(0.5-0.5*math.Cos(m.now*2.4))*0.45))
	}
	switch {
	case d.busyPhase():
		g, gc = '◌', dots.Lerp(cMist, cSignal, float32(0.5+0.5*math.Sin(m.now*7)))
	case (d.phase == phaseRunning || d.phase == phaseReady) && !d.stopErr && m.now-d.phaseAt < flashDur && d.phaseAt > d.born:
		k := 1 - (m.now-d.phaseAt)/flashDur
		gc = dots.Lerp(gc, cSignal, float32(k))
		if k > 0.5 {
			g = '◉'
		}
	}
	m.print(w.inL, w.headRow, string(g), gc, p)
	m.write(w.inL+2, w.headRow, w.inR, p,
		span{c.Name, cPaper}, span{"  " + shortID(c.ID), cMist}, span{"  " + c.Image, dots.Lerp(cMist, cPaper, 0.55)})

	label := func(s string) span { return span{s + "  ", cMist} }
	gap := span{"     ", cMist}
	parts := []span{label("STATE"), {look.Word, dots.Hex(look.Color)}, gap, label("STATUS"), {c.Status, cPaper}}
	if c.Ports != "" {
		parts = append(parts, gap, label("PORTS"), span{c.Ports, cPaper})
	}
	m.write(w.inL+2, w.stateRow, w.inR, p, parts...)
}

// drawDialogAction is the action button (start, stop, retry, or its
// progress while Docker works) and what is happening, beside it.
func (m *Model) drawDialogAction(w dlgLayout, p float64) {
	cv, d := m.cv, m.dlg
	name := d.c.Name
	b := w.act
	say := func(parts ...span) { m.write(b[1]+3, b[2], w.inR, p, parts...) }

	if d.busyPhase() {
		// The button turns into a run of halftone dots while Docker works.
		for col := b[0]; col < b[1]; col++ {
			cv.Back(col, b[2], cLift)
			v := 0.5
			if !m.reduce {
				v = 0.5 + 0.5*math.Sin(float64(col-b[0])*0.9-m.now*10)
			}
			cv.Glyph(col, b[2], ramp[min(2, int(v*3))], dots.Lerp(cAsh, cSignal, float32(v)))
		}
		if d.phase == phaseStarting {
			say(span{"starting " + name + "…", cPaper})
		} else {
			say(span{"stopping " + name + "…", cPaper}, span{"  waiting for it to exit", cMist})
		}
		return
	}

	// Start is the loud button; stop is quieter, it ends something that works.
	label, bg, fg := startLabel, cSignal, cNight
	hot := dots.Lerp(cSignal, cPaper, 0.3)
	switch d.phase {
	case phaseFailed:
		label = retryLabel
	case phaseRunning:
		label, bg, fg, hot = stopLabel, cRaise, cPaper, cLift
	}
	if d.hover == 1 {
		bg = hot
	}
	for col := b[0]; col < b[1]; col++ {
		cv.Back(col, b[2], bg)
	}
	m.write(b[0], b[2], b[1], p, span{label, fg})
	if d.phase == phaseRunning {
		cv.Glyph(b[0]+1, b[2], '■', cSignal)
	}

	switch {
	case d.phase == phaseFailed:
		say(span{"Docker didn't start it.", cSignal}, span{"  The reason is below.", cMist})
	case d.stopErr:
		say(span{"Docker didn't stop it.", cSignal}, span{"  The reason is below.", cMist})
	case d.phase == phaseRunning && d.opening:
		say(span{"opening its output…", cMist})
	case d.phase == phaseRunning && d.ended:
		say(span{"its output ended", cMist})
	case d.phase == phaseRunning:
		say(span{"following its output", cMist})
	default:
		say(span{name + " is stopped.", cPaper}, span{"  Its output shows below.", cMist})
	}
}

// drawTerminal is the inset where the output scrolls.
func (m *Model) drawTerminal(w dlgLayout) {
	cv, d := m.cv, m.dlg
	for row := w.termTop; row < w.termTop+w.termRows; row++ {
		for col := w.x0 + 2; col <= w.x1-2; col++ {
			cv.Back(col, row, cNight)
		}
	}
	tw := w.termR - w.termL
	vis, back := d.tail(tw, w.termRows)
	d.back = back

	if len(vis) == 0 {
		msg := "nothing yet. What it prints shows up here."
		switch {
		case d.phase == phaseStarting:
			msg = "waiting for Docker…"
		case d.opening:
			msg = "waiting for output…"
		case d.ended:
			msg = "it printed nothing."
		}
		m.write(w.termL, w.termTop, w.termR, 1, span{msg, cAsh})
	}

	out := dots.Lerp(cMist, cPaper, 0.8)
	for i, r := range vis {
		y := w.termTop + i
		fg := out
		switch r.kind {
		case stderrLine:
			if !r.cont {
				cv.Glyph(w.termL-2, y, '▎', dots.Lerp(cAsh, cSignal, 0.55))
			}
			fg = dots.Lerp(cMist, cPaper, 0.6)
		case noteLine:
			if !r.cont {
				cv.Glyph(w.termL-2, y, '!', cSignal)
			}
			fg = cSignal
		case infoLine:
			fg = cMist
		}
		pr := 1.0
		if !m.reduce {
			pr = (m.now - r.at) / lineIn
		}
		m.print(w.termL, y, r.text, fg, pr)
	}

	// The bottom edge says where you are in the output.
	var st []span
	n := len(d.logs)
	switch {
	case back > 0:
		st = []span{{" ↑ ", cSignal}, {strconv.Itoa(back) + " rows back   ", cPaper}, {"G", cSignal}, {" follow ", cMist}}
	case n > 0 && d.stream != nil:
		st = []span{{" " + plural(n, "line") + " · following ", cMist}}
	case n > 0:
		st = []span{{" " + plural(n, "line") + " ", cMist}}
	}
	m.write(w.x0+2, w.y1, w.x1-2, 1, st...)

	// A scrollbar when there is more than fits.
	if total := d.rowCount(tw); total > w.termRows {
		thumb := max(1, w.termRows*w.termRows/total)
		at := (total - w.termRows - back) * (w.termRows - thumb) / (total - w.termRows)
		for i := 0; i < w.termRows; i++ {
			g, c := '│', cAsh
			if i >= at && i < at+thumb {
				g, c = '┃', cMist
			}
			cv.Glyph(w.x1-2, w.termTop+i, g, c)
		}
	}
}

// rowCount is how many terminal rows all the lines take at width w.
func (d *dialog) rowCount(w int) int {
	n := 0
	for _, l := range d.logs {
		n += len(wrap(l.text, w))
	}
	return n
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
