package dashboard

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"docker-tool/internal/screen/dots"
	"docker-tool/internal/screen/theme"
)

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
	title, area := " C O N T A I N E R ", "OUTPUT"
	if d.kind == imageDlg {
		title, area = " I M A G E ", "DETAILS"
	}
	m.write(w.x0+2, w.y0, w.close[0]-1, 1, span{title, cPaper})
	cc := cMist
	if d.onClose {
		cc = cPaper
		for col := w.close[0]; col < w.close[1]; col++ {
			cv.Back(col, w.y0, cLift)
		}
	}
	m.write(w.close[0], w.y0, w.close[1], 1, span{closeLabel, cc})

	if d.kind == imageDlg {
		m.drawImageHead(w, p)
	} else {
		m.drawContainerHead(w, p)
		m.drawStats(w, p)
	}
	m.drawActions(w, p)

	m.write(w.inL, w.ruleRow, w.inR, p, span{area, cMist})
	for col := w.inL + len(area) + 2; col < w.inR; col += 2 {
		cv.Glyph(col, w.ruleRow, '·', cFrame)
	}
	m.drawTerminal(w)
}

// drawContainerHead is the container's name, image and state.
func (m *Model) drawContainerHead(w dlgLayout, p float64) {
	d := m.dlg
	c := d.c
	look := theme.ContainerLook(c.State, c.Health)
	g, gc := look.Glyph, dots.Hex(look.Color)
	if look.Word == "running" {
		amb := m.ambient()
		gc = dots.Lerp(cMist, cPaper, float32(1-amb*(0.5-0.5*math.Cos(m.now*2.4))*0.45))
	}
	switch {
	case d.busy():
		g, gc = '◌', dots.Lerp(cMist, cSignal, float32(0.5+0.5*math.Sin(m.now*7)))
	case (d.phase == phaseRunning || d.phase == phaseReady) && d.failMsg == "" && m.now-d.phaseAt < flashDur && d.phaseAt > d.born:
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

// drawImageHead is the image's name, size, age and how many names it has.
func (m *Model) drawImageHead(w dlgLayout, p float64) {
	d := m.dlg
	im := d.img
	g := '◆'
	gc := dots.Lerp(cMist, cPaper, 0.6)
	if d.busy() {
		g, gc = '◇', dots.Lerp(cMist, cSignal, float32(0.5+0.5*math.Sin(m.now*7)))
	}
	m.print(w.inL, w.headRow, string(g), gc, p)
	m.write(w.inL+2, w.headRow, w.inR, p, span{imageName(im), cPaper}, span{"  " + im.ShortID, cMist})

	label := func(s string) span { return span{s + "  ", cMist} }
	gap := span{"     ", cMist}
	created := "unknown"
	if !im.CreatedAt.IsZero() {
		created = ago(im.CreatedAt, m.wall)
	}
	m.write(w.inL+2, w.stateRow, w.inR, p,
		label("SIZE"), span{humanSize(im.Size), cPaper}, gap,
		label("CREATED"), span{created, cPaper}, gap,
		label("TAGS"), span{strconv.Itoa(len(im.Tags)), cPaper})
}

// drawActions is the row of buttons (or the halftone progress that stands
// in for them while Docker works) and what is happening, beside them.
func (m *Model) drawActions(w dlgLayout, p float64) {
	cv, d := m.cv, m.dlg
	// say writes the message beside the buttons; when it doesn't fit, only
	// its first part.
	say := func(parts ...span) {
		if w.sayX+width(parts) > w.inR {
			parts = parts[:1]
		}
		m.write(w.sayX, w.actRow, w.inR, p, parts...)
	}
	name := d.c.Name
	if d.kind == imageDlg {
		name = imageName(d.img)
	}

	if d.busy() {
		for col := w.inL; col < w.inL+busyWidth; col++ {
			cv.Back(col, w.actRow, cLift)
			v := 0.5
			if !m.reduce {
				v = 0.5 + 0.5*math.Sin(float64(col-w.inL)*0.9-m.now*10)
			}
			cv.Glyph(col, w.actRow, ramp[min(2, int(v*3))], dots.Lerp(cAsh, cSignal, float32(v)))
		}
		verb := map[phase]string{phaseStarting: "starting ", phaseStopping: "stopping ",
			phaseRestarting: "restarting ", phaseRemoving: "removing "}[d.phase]
		switch d.phase {
		case phaseStopping, phaseRestarting:
			say(span{verb + name + "…", cPaper}, span{"  waiting for it to exit", cMist})
		default:
			say(span{verb + name + "…", cPaper})
		}
		return
	}

	for _, b := range w.btns {
		bg, fg := cRaise, cPaper
		if b.loud {
			bg, fg = cSignal, cNight
		}
		if d.hover == b.act {
			bg = dots.Lerp(bg, cPaper, 0.22)
		}
		for col := b.x0; col < b.x1; col++ {
			cv.Back(col, w.actRow, bg)
		}
		m.write(b.x0, w.actRow, b.x1, p, span{b.label, fg})
		if b.icon != 0 && !b.loud {
			ic := cSignal
			if b.act == actRestart || b.act == actUntag {
				ic = cMist
			}
			m.print(b.x0+1, w.actRow, string(b.icon), ic, p)
		}
	}

	why := span{"  The reason is below.", cMist}
	switch {
	case d.armed == actRemove && d.kind == containerDlg:
		say(span{"Remove " + name + "?", cSignal}, span{"  Its volumes are kept.", cMist})
	case d.armed == actRemove && len(d.img.Tags) > 1:
		say(span{"Remove the image and its " + strconv.Itoa(len(d.img.Tags)) + " tags?", cSignal}, span{"  esc keeps it.", cMist})
	case d.armed == actRemove:
		say(span{"Remove " + name + " for good?", cSignal}, span{"  esc keeps it.", cMist})
	case d.blocker != "":
		user := "a container"
		if c, ok := m.containerByID(d.blocker); ok {
			user = c.Name
		}
		say(span{user + " uses this image.", cSignal}, span{"  Remove that container first.", cMist})
	case d.failMsg != "":
		say(span{d.failMsg, cSignal}, why)
	case d.kind == imageDlg && len(d.img.Tags) > 1:
		say(span{"untag drops only " + d.img.Tags[0], cMist})
	case d.kind == imageDlg:
		say(span{"Removing frees up to " + humanSize(d.img.Size) + ".", cMist})
	case d.phase == phaseRunning && d.opening:
		say(span{"opening its output…", cMist})
	case d.phase == phaseRunning && d.ended:
		say(span{"its output ended", cMist})
	case d.phase == phaseRunning:
		say(span{"following its output", cMist})
	default:
		say(span{"stopped", cMist})
	}
}

// drawTerminal is the inset where the output (or an image's details)
// scrolls.
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
		case d.phase == phaseStarting, d.phase == phaseRestarting:
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
	case d.kind == imageDlg:
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

// vrow is one panel row: a log line, or a piece of a wrapped one.
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

// rowCount is how many panel rows all the lines take at width w.
func (d *dialog) rowCount(w int) int {
	n := 0
	for _, l := range d.logs {
		n += len(wrap(l.text, w))
	}
	return n
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

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
