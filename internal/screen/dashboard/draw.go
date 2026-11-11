package dashboard

import (
	"fmt"
	"math"
	"strings"

	"docker-tool/internal/screen/dots"
)

const wordmark = "A N C H O R S"

// Halftone glyphs from smallest to largest, as on the splash.
var ramp = [...]rune{'·', '•', '●'}

func (m *Model) View() string {
	if m.cv == nil {
		return ""
	}
	if m.cv.Cols < minCols || m.cv.Rows < minRows {
		return fmt.Sprintf("anchors needs a terminal of at least %dx%d (this one is %dx%d)",
			minCols, minRows, m.cv.Cols, m.cv.Rows)
	}
	m.cv.Clear()
	top, bot := m.drawBox()
	hints := m.drawFooter()
	m.drawSea(top, bot, hints)
	m.drawShip(top, bot)
	return m.cv.Render(cNight)
}

// span is a run of text in one color.
type span struct {
	s  string
	fg dots.RGB
}

func width(parts []span) int {
	n := 0
	for _, s := range parts {
		n += len([]rune(s.s))
	}
	return n
}

// write draws spans from col, clipped before limit, and returns where they end.
func (m *Model) write(col, row, limit int, p float64, parts ...span) int {
	for _, s := range parts {
		r := []rune(s.s)
		if col+len(r) > limit {
			r = []rune(clip(s.s, limit-col))
		}
		col = m.print(col, row, string(r), s.fg, p)
		if col >= limit {
			break
		}
	}
	return col
}

// print writes s at col,row. While p < 1 the text is still printing in:
// each letter first shows as a halftone dot, then as itself.
func (m *Model) print(col, row int, s string, fg dots.RGB, p float64) int {
	for _, r := range s {
		switch {
		case p >= 1:
			m.cv.Glyph(col, row, r, fg)
		case r == ' ':
		default:
			h := hash(col, row)
			if h < p*1.35-0.35 {
				m.cv.Glyph(col, row, r, fg)
			} else if h < p*1.35 {
				m.cv.Glyph(col, row, '·', dots.Lerp(cAsh, fg, 0.4))
			}
		}
		col++
	}
	return col
}

// unfold is how far the box has opened, 0..1.
func (m *Model) unfold() float64 {
	if m.reduce {
		return 1
	}
	return smooth(0, boxOpen, m.now)
}

// drawSea fills the screen around the box with the halftone: it swells
// while you work, thins out toward the box so the box floats on a calm
// patch, parts under the cursor and carries the wake and ripples.
// top..bot is the part of the box that is open; hints is the footer's span.
func (m *Model) drawSea(top, bot int, hints [2]int) {
	cv, l := m.cv, m.lay
	R := m.reach()
	dev := 1.0
	if !m.reduce {
		dev = smooth(0, 0.5, m.now)
	}
	bx0, bx1 := float64(l.bx0), float64(l.bx1+1)
	by0, by1 := float64(top)*2, float64(bot+1)*2
	for row := 0; row < cv.Rows; row++ {
		for col := row % 2; col < cv.Cols; col += 2 {
			if col >= l.bx0-1 && col <= l.bx1+1 && row >= top && row <= bot {
				continue
			}
			if row == l.helpRow && col >= hints[0]-2 && col < hints[1]+2 {
				continue
			}
			p := vec{float64(col) + 0.5, float64(row)*2 + 1}

			// Distance to the box, in units, for the calm patch around it.
			dx := math.Max(0, math.Max(bx0-p.X, p.X-bx1))
			dy := math.Max(0, math.Max(by0-p.Y, p.Y-by1))
			calm := 0.35 + 0.65*smooth(0.5, 6, math.Hypot(dx, dy))

			src, lens := p, 0.0
			if m.lensOn > 0 {
				if d := math.Hypot(p.X-m.lens.X, p.Y-m.lens.Y); d < R {
					lens = (1 - d/R) * (1 - d/R) * m.lensOn
					if !m.reduce {
						src = vec{m.lens.X + (p.X-m.lens.X)*(1-0.45*lens), m.lens.Y + (p.Y-m.lens.Y)*(1-0.45*lens)}
					}
				}
			}
			t := m.swellT
			swell := 0.5 + 0.5*math.Sin(src.X*0.12-t*1.5+1.7*math.Sin(src.Y*0.08+src.X*0.025+t*0.45)+src.Y*0.19)
			v := (0.16 + 0.54*swell) * calm * dev

			heat := m.heat[row*cv.Cols+col]
			wave := m.wave(p)
			v += lens*0.25 + heat*0.55 + wave*0.6
			if v < 0.08 {
				continue
			}
			g := ramp[0]
			switch {
			case v > 0.62:
				g = ramp[2]
			case v > 0.34:
				g = ramp[1]
			}
			c := dots.Lerp(dots.Lerp(cAsh, cMist, 0.35), dots.Lerp(cMist, cPaper, 0.4), float32(smooth(0.1, 0.9, v)))
			c = dots.Lerp(c, cSignal, float32(math.Min(1, lens*0.8+heat*1.2+wave)*0.9))
			cv.Glyph(col, row, g, c)
		}
	}
}

// drawBox draws the manifest and returns the rows that are open.
func (m *Model) drawBox() (top, bot int) {
	cv, l := m.cv, m.lay
	for row := l.by0; row <= l.by1; row++ {
		for col := l.bx0; col <= l.bx1; col++ {
			cv.Back(col, row, cPanel)
		}
	}
	m.drawHeader()
	m.drawTabs()
	m.drawList()
	if m.showKeys {
		m.drawKeys()
	} else {
		m.drawDetail()
	}

	// The box unfolds from its middle: rows outside the open part are
	// cleared, and the frame is drawn at the edges of what is open.
	h := l.by1 - l.by0 + 1
	open := max(2, int(math.Round(float64(h)*m.unfold())))
	top = l.by0 + (h-open)/2
	bot = top + open - 1
	for row := l.by0; row <= l.by1; row++ {
		if row < top || row > bot {
			for col := l.bx0; col <= l.bx1; col++ {
				cv.ClearCell(col, row)
			}
		}
	}
	m.drawFrame(top, bot)
	return top, bot
}

// drawFrame is the hairline border, with the name set into the top edge and
// the sync state into the bottom one.
func (m *Model) drawFrame(top, bot int) {
	cv, l := m.cv, m.lay
	for col := l.bx0 + 1; col < l.bx1; col++ {
		cv.Glyph(col, top, '─', cFrame)
		cv.Glyph(col, bot, '─', cFrame)
	}
	for row := top + 1; row < bot; row++ {
		cv.Glyph(l.bx0, row, '│', cFrame)
		cv.Glyph(l.bx1, row, '│', cFrame)
	}
	cv.Glyph(l.bx0, top, '╭', cFrame)
	cv.Glyph(l.bx1, top, '╮', cFrame)
	cv.Glyph(l.bx0, bot, '╰', cFrame)
	cv.Glyph(l.bx1, bot, '╯', cFrame)
	if top < l.ruleRow && bot > l.ruleRow {
		cv.Glyph(l.bx0, l.ruleRow, '├', cFrame)
		cv.Glyph(l.bx1, l.ruleRow, '┤', cFrame)
	}
	if m.unfold() < 0.7 {
		return
	}

	show := 1.0
	if !m.reduce {
		show = smooth(boxOpen*0.7, boxOpen+0.4, m.now)
	}
	m.write(l.bx0+2, top, l.bx1-2, show, span{" " + wordmark + " ", cPaper})
	if m.host != "" {
		host := " " + clip(m.host, (l.bx1-l.bx0)/2) + " "
		m.write(l.bx1-2-len([]rune(host)), top, l.bx1-1, 1, span{host, cMist})
	}

	var sync []span
	switch {
	case m.loading > 0:
		g := [...]rune{'·', '•', '●', '•'}[int(m.now*8)%4]
		sync = []span{{" " + string(g) + " syncing ", cMist}}
	case m.cErr != nil || m.iErr != nil:
		sync = []span{{" docker unreachable ", cSignal}}
	case !m.synced.IsZero():
		sync = []span{{" synced " + m.synced.Format("15:04:05") + " ", cMist}}
	}
	m.write(l.bx0+2, bot, l.bx1-2, 1, sync...)
	if leg := m.legend(); leg != nil && width(sync)+width(leg)+8 < l.bx1-l.bx0 {
		m.write(l.bx1-2-width(leg), bot, l.bx1-1, 1, leg...)
	}
}

// drawHeader is the summary line at the top of the box.
func (m *Model) drawHeader() {
	l := m.lay
	m.write(l.glyphX, l.summaryRow, l.inR, 1, m.summary()...)
}

// summary is the counts: "4 running · 2 need a look · 11 stopped · 9 images".
func (m *Model) summary() []span {
	switch {
	case m.cErr != nil && !m.cLoaded:
		return []span{{"docker unreachable", cSignal}}
	case !m.cLoaded:
		return []span{{"connecting to docker", cMist}}
	}
	up, down, trouble := 0, 0, 0
	for _, c := range m.containers {
		switch c.State {
		case "running":
			up++
			if c.Health == "unhealthy" {
				trouble++
			}
		case "restarting", "dead":
			trouble++
		default:
			down++
		}
	}
	out := []span{{fmt.Sprint(up), cPaper}, {" running", cMist}}
	if trouble > 0 {
		out = append(out, span{"  ·  ", cAsh}, span{fmt.Sprint(trouble), cSignal}, span{" need a look", cMist})
	}
	out = append(out, span{"  ·  ", cAsh}, span{fmt.Sprint(down), cPaper}, span{" stopped", cMist})
	if m.iLoaded {
		out = append(out, span{"  ·  ", cAsh}, span{fmt.Sprint(len(m.images)), cPaper}, span{" images", cMist})
	}
	return out
}

func (m *Model) count(s section) string {
	switch s {
	case containersTab:
		if m.cLoaded {
			return fmt.Sprint(len(m.containers))
		}
	case imagesTab:
		if m.iLoaded {
			return fmt.Sprint(len(m.images))
		}
	}
	return "–"
}

// tabSpans is where each tab's label sits on the tab row, [from, to).
func (m *Model) tabSpans() [numTabs][2]int {
	var out [numTabs][2]int
	x := m.lay.glyphX
	for t := range numTabs {
		w := len(tabNames[t]) + 1 + len(m.count(t))
		out[t] = [2]int{x, x + w}
		x += w + 5
	}
	return out
}

func (m *Model) drawTabs() {
	cv, l := m.cv, m.lay
	for t, s := range m.tabSpans() {
		name := cMist
		if section(t) == m.sec {
			name = cPaper
		}
		m.write(s[0], l.tabRow, l.inR, 1, span{tabNames[t], name}, span{" " + m.count(section(t)), cMist})
	}
	if m.filter != "" && !m.filtering {
		parts := []span{{"/ ", cSignal}, {clip(m.filter, 20), cPaper}, {"   esc clear", cMist}}
		m.write(l.inR-width(parts), l.tabRow, l.inR, 1, parts...)
	}

	for c := l.bx0 + 1; c < l.bx1; c++ {
		cv.Glyph(c, l.ruleRow, '─', cFrame)
	}
	for c := int(math.Round(m.tabX[0])); c < int(math.Round(m.tabX[1])); c++ {
		cv.Glyph(c, l.ruleRow, '━', cSignal)
	}
}

func (m *Model) columns() []placed {
	l := m.lay
	return place(sectionColumns[m.sec], l.textX, l.inR-l.textX)
}

// rowProgress is how far a list row has printed in (1 = done). Nothing
// prints before the box has opened.
func (m *Model) rowProgress(id string, vi int) float64 {
	if m.reduce {
		return 1
	}
	b := math.Max(m.sectionBorn, boxOpen*0.8) + float64(vi)*printStagger
	if t, ok := m.born[id]; ok && t > b {
		b = t
	}
	return (m.now - b) / printDur
}

func (m *Model) drawList() {
	cv, l := m.cv, m.lay
	cols := m.columns()
	for _, c := range cols {
		x := c.x
		if c.right {
			x = c.x + c.w - len(c.title)
		}
		m.write(x, l.headRow, l.inR, 1, span{c.title, cMist})
	}

	rows := m.rows(m.sec)
	loaded, err := m.cLoaded, m.cErr
	if m.sec == imagesTab {
		loaded, err = m.iLoaded, m.iErr
	}
	switch {
	case m.sec == podsTab:
		m.note(span{"Kubernetes isn't connected yet.", cPaper},
			span{"Pods will line up here, next to your containers, once anchors reads your kubeconfig.", cMist})
		return
	case err != nil && !loaded:
		m.note(span{"Docker isn't answering.", cPaper},
			span{err.Error(), cMist},
			span{"", cMist},
			span{"Start Docker, or point DOCKER_HOST at it, then press r.", cMist})
		return
	case !loaded:
		m.skeleton(cols)
		return
	case len(rows) == 0 && m.filter != "":
		m.note(span{"Nothing matches “" + m.filter + "”.", cPaper},
			span{"esc clears the filter.", cMist})
		return
	case len(rows) == 0 && m.sec == containersTab:
		m.note(span{"No containers on this machine yet.", cPaper},
			span{"Start one, for example  docker run -d nginx , and it shows up here within seconds.", cMist})
		return
	case len(rows) == 0:
		m.note(span{"No images yet.", cPaper},
			span{"Pull one, for example  docker pull nginx , and it shows up here within seconds.", cMist})
		return
	}

	off := m.off[m.sec]
	r0 := int(math.Floor(m.visPos))
	f := m.visPos - float64(r0)
	amb := m.ambient()
	breath := 1 - amb*(0.5-0.5*math.Cos(m.now*2.4))*0.45

	for vi := 0; vi < l.listRows && off+vi < len(rows); vi++ {
		r := rows[off+vi]
		y := l.listTop + vi
		sel := 0.0
		switch vi {
		case r0:
			sel = 1 - f
		case r0 + 1:
			sel = f
		}
		bg := cPanel
		if vi == m.hover {
			bg = cRaise
		}
		bg = dots.Lerp(bg, cLift, float32(sel))
		for c := l.bx0 + 1; c < l.bx1; c++ {
			cv.Back(c, y, bg)
		}
		p := m.rowProgress(r.id, vi)

		if r.look.Glyph != 0 {
			g, gc := r.look.Glyph, dots.Hex(r.look.Color)
			if r.look.Word == "running" {
				gc = dots.Lerp(cMist, cPaper, float32(breath))
			}
			if t, ok := m.flash[r.id]; ok && m.now-t < flashDur {
				k := 1 - (m.now-t)/flashDur
				gc = dots.Lerp(gc, cSignal, float32(k))
				if k > 0.5 {
					g = '◉'
				}
			}
			m.print(l.glyphX, y, string(g), gc, p)
		}

		for _, c := range cols {
			text := clip(r.cells[c.idx], c.w)
			x := c.x
			if c.right {
				x = c.x + c.w - len([]rune(text))
			}
			m.print(x, y, text, m.cellColor(r, c.idx, sel), p)
		}
	}

	// The selection bar: Braille dots, so it glides in quarter-row steps.
	y0 := int(math.Round((float64(l.listTop) + m.visPos) * 4))
	for dy := 0; dy < 4; dy++ {
		cv.Dot(l.markX*2, y0+dy, cSignal, 3)
		cv.Dot(l.markX*2+1, y0+dy, cSignal, 3)
	}

	// Scrollbar, only when the list doesn't fit.
	if n := len(rows); n > l.listRows {
		thumb := max(1, l.listRows*l.listRows/n)
		at := off * (l.listRows - thumb) / (n - l.listRows)
		for i := 0; i < l.listRows; i++ {
			g, c := '│', cFrame
			if i >= at && i < at+thumb {
				g, c = '┃', cMist
			}
			cv.Glyph(l.bx1-1, l.listTop+i, g, c)
		}
	}
}

// cellColor: names in paper, supporting columns quieter, state in its look.
// The selected row (sel near 1) lifts everything toward paper.
func (m *Model) cellColor(r row, idx int, sel float64) dots.RGB {
	half := dots.Lerp(cMist, cPaper, 0.45)
	var c dots.RGB
	switch {
	case r.dim:
		c = cMist
	case r.c != nil:
		c = [...]dots.RGB{cPaper, half, dots.Hex(r.look.Color), cMist, half}[idx]
	case r.img != nil:
		c = [...]dots.RGB{cPaper, dots.Lerp(cMist, cPaper, 0.7), cMist, half, cMist}[idx]
	}
	if c != cSignal {
		c = dots.Lerp(c, cPaper, float32(sel*0.4))
	}
	return c
}

// note is a short message in the list area, for empty and error states.
func (m *Model) note(lines ...span) {
	l := m.lay
	p := m.rowProgress("", 0)
	for i, s := range lines {
		if i >= l.listRows {
			break
		}
		m.write(l.textX, l.listTop+1+i, l.inR, p, s)
	}
}

// skeleton is the list while the first load is out: halftone bars with a
// shimmer running through them.
func (m *Model) skeleton(cols []placed) {
	l := m.lay
	for vi := 0; vi < min(l.listRows, 6); vi++ {
		y := l.listTop + vi
		for _, c := range cols {
			n := int(float64(c.w) * (0.3 + 0.5*hash(c.idx, vi)))
			for x := 0; x < n; x += 2 {
				col := c.x + x
				sh := 0.5 + 0.5*math.Sin((float64(col)-m.now*45)*0.06+float64(vi)*0.5)
				m.cv.Glyph(col, y, '·', dots.Lerp(cAsh, cMist, float32(sh*0.85)))
			}
		}
	}
}

// dottedRule is a line of halftone dots across the box.
func (m *Model) dottedRule(row int) {
	l := m.lay
	for c := l.bx0 + 2; c < l.bx1-1; c += 2 {
		m.cv.Glyph(c, row, '·', cFrame)
	}
}

// drawDetail is the panel at the bottom of the box: the selection in full.
func (m *Model) drawDetail() {
	l := m.lay
	m.dottedRule(l.detailRule)
	r, ok := m.selected()
	if !ok || m.sec == podsTab {
		return
	}
	p := 1.0
	if !m.reduce {
		p = (m.now - math.Max(m.detailBorn, boxOpen*0.8)) / 0.18
	}
	x, end, y := l.glyphX, l.inR, l.detailTop
	label := func(s string) span { return span{s + "  ", cMist} }
	gap := span{"     ", cMist}

	switch {
	case r.c != nil:
		c := r.c
		m.write(x, y, end, p, span{c.Name, cPaper}, span{"  " + shortID(c.ID), cMist}, span{"  " + c.Image, dots.Lerp(cMist, cPaper, 0.55)})
		health := c.Health
		if health == "" {
			health = "no healthcheck"
		}
		m.write(x, y+1, end, p,
			label("STATE"), span{string(r.look.Glyph) + " " + r.look.Word, dots.Hex(r.look.Color)}, gap,
			label("HEALTH"), span{health, cPaper})
		m.write(x, y+2, end, p, label("STATUS"), span{c.Status, cPaper})
		ports := c.Ports
		if ports == "" {
			ports = "none published"
		}
		m.write(x, y+3, end, p, label("PORTS"), span{ports, cPaper})

	case r.img != nil:
		im := r.img
		name := "<none>"
		if len(im.Tags) > 0 {
			name = im.Tags[0]
		}
		m.write(x, y, end, p, span{name, cPaper}, span{"  " + im.ShortID, cMist})
		created := "unknown"
		if !im.CreatedAt.IsZero() {
			created = ago(im.CreatedAt, m.wall) + " · " + im.CreatedAt.Format("2006-01-02")
		}
		m.write(x, y+1, end, p, label("SIZE"), span{humanSize(im.Size), cPaper}, gap, label("CREATED"), span{created, cPaper})
		next := y + 2
		switch {
		case len(im.Tags) > 1:
			m.write(x, next, end, p, label("ALSO"), span{strings.Join(im.Tags[1:], ", "), cPaper})
			next++
		case len(im.Tags) == 0:
			m.write(x, next, end, p, label("UNTAGGED"), span{"a dangling image, safe to prune if no container uses it", cMist})
			next++
		}
		if len(im.Digests) > 0 {
			m.write(x, next, end, p, label("DIGEST"), span{im.Digests[0], cMist})
		}
	}
}

// drawKeys replaces the detail panel with every key binding.
func (m *Model) drawKeys() {
	l := m.lay
	m.dottedRule(l.detailRule)
	colW := min(30, (l.inR-l.glyphX)/len(m.keys.FullHelp()))
	for ci, group := range m.keys.FullHelp() {
		x := l.glyphX + ci*colW
		for ri, b := range group {
			h := b.Help()
			m.write(x, l.detailTop+ri, min(x+colW-1, l.inR), 1,
				span{fmt.Sprintf("%-5s", h.Key), cSignal}, span{" " + h.Desc, cMist})
		}
	}
}

// drawFooter centers the key hints (or the filter input) under the box and
// returns the columns they cover, so the sea leaves them a clear strip.
func (m *Model) drawFooter() [2]int {
	cv, l := m.cv, m.lay
	if m.unfold() < 1 {
		return [2]int{}
	}
	var parts []span
	if m.filtering {
		parts = []span{{"/ ", cSignal}, {m.filter, cPaper}, {"▏", cSignal}, {"    ", cMist},
			{"enter", cSignal}, {" keep   ", cMist}, {"esc", cSignal}, {" clear", cMist}}
		if int(m.now*2)%2 == 1 && !m.reduce {
			parts[2].fg = cPanel // the caret blinks while the screen is awake
		}
	} else {
		for _, b := range m.keys.ShortHelp() {
			h := b.Help()
			if len(parts) > 0 {
				parts = append(parts, span{"    ", cMist})
			}
			parts = append(parts, span{h.Key, cSignal}, span{" " + h.Desc, cMist})
		}
	}
	w := min(width(parts), cv.Cols-4)
	x := max(2, min(cv.Cols-2-w, (l.bx0+l.bx1+1)/2-w/2))
	m.write(x, l.helpRow, x+w, 1, parts...)
	return [2]int{x, x + w}
}

func shortID(id string) string {
	if _, rest, ok := strings.Cut(id, ":"); ok {
		id = rest
	}
	if len(id) > 12 {
		id = id[:12]
	}
	return id
}
