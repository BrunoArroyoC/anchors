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

const (
	markCol  = 1 // the selection bar
	glyphCol = 3 // a row's state glyph
	textCol  = 5 // where the columns start
)

func (m *Model) View() string {
	if m.cv == nil {
		return ""
	}
	if m.cv.Cols < minCols || m.cv.Rows < minRows {
		return fmt.Sprintf("anchors needs a terminal of at least %dx%d (this one is %dx%d)",
			minCols, minRows, m.cv.Cols, m.cv.Rows)
	}
	m.cv.Clear()
	m.drawBand()
	m.drawTabs()
	m.drawList()
	if m.showKeys {
		m.drawKeys()
	} else {
		m.drawDetail()
	}
	m.drawFooter()
	return m.cv.Render(cNight)
}

// span is a run of text in one color.
type span struct {
	s  string
	fg dots.RGB
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

// drawBand is the poster strip across the top: the wordmark on the left, a
// halftone sea that swells while you work and parts under the cursor, and
// the counts on the right.
func (m *Model) drawBand() {
	cv := m.cv
	// Counts and host, right-aligned, decide where the halftone stops.
	summary := m.summary()
	sumW := 0
	for _, s := range summary {
		sumW += len([]rune(s.s))
	}
	host := m.host
	stop := cv.Cols - 2 - max(sumW, len([]rune(host))) - 3
	start := 2 + len(wordmark) + 4

	dev := 1.0
	if !m.reduce {
		dev = smooth(0, 0.9, m.now)
	}
	R := m.reach()
	for row := 0; row < bandRows; row++ {
		for col := start + (start+row)%2; col < stop; col += 2 {
			p := vec{float64(col) + 0.5, float64(row)*2 + 1}
			src, lens := p, 0.0
			if m.lensOn > 0 {
				if d := math.Hypot(p.X-m.lens.X, p.Y-m.lens.Y); d < R {
					lens = (1 - d/R) * (1 - d/R) * m.lensOn
					if !m.reduce {
						src = vec{m.lens.X + (p.X-m.lens.X)*(1-0.45*lens), m.lens.Y + (p.Y-m.lens.Y)*(1-0.45*lens)}
					}
				}
			}
			// The sea: denser away from the wordmark, thinning at both ends.
			across := (src.X - float64(start)) / math.Max(1, float64(stop-start))
			edge := smooth(0, 0.35, across) * (1 - 0.6*smooth(0.8, 1, across))
			swell := 0.5 + 0.5*math.Sin(src.X*0.16-m.swellT*1.7+1.8*math.Sin(src.X*0.041+m.swellT*0.55)+src.Y*0.45)
			v := edge * (0.16 + 0.62*swell) * dev

			heat := m.heat[row*cv.Cols+col]
			wave := 0.0
			for _, r := range m.ripples {
				d := math.Hypot(p.X-r.at.X, p.Y-r.at.Y)
				wave += math.Exp(-math.Pow((d-r.age*70)/3, 2)) * (1 - r.age/1.8)
			}
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
			c := dots.Lerp(cAsh, dots.Lerp(cMist, cPaper, 0.6), float32(smooth(0.05, 0.85, v)))
			c = dots.Lerp(c, cSignal, float32(math.Min(1, lens*0.8+heat*1.2+wave)*0.9))
			cv.Glyph(col, row, g, c)
		}
	}

	// The name, tracked out like the poster's labels, prints in from dots.
	show := 1.0
	if !m.reduce {
		show = smooth(0.05, 0.6, m.now)
	}
	m.print(2, 1, wordmark, cPaper, show)

	sx := cv.Cols - 2 - sumW
	m.write(sx, 1, cv.Cols-2, 1, summary...)
	hx := cv.Cols - 2 - len([]rune(host))
	if host != "" {
		m.write(max(stop+3, hx), 2, cv.Cols-2, 1, span{host, cMist})
	}
}

// summary is the counts in the band: "4 running · 2 stopped · 9 images".
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
	if m.cv != nil && m.cv.Cols < 100 && len(out) > 6 {
		out = out[:len(out)-3] // keep the band breathing room on small terminals
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
	x := 2
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
		m.write(s[0], l.tabRow, cv.Cols, 1, span{tabNames[t], name}, span{" " + m.count(section(t)), cMist})
	}
	if m.filter != "" && !m.filtering {
		f := clip(m.filter, 24)
		x := cv.Cols - 2 - len([]rune(f)) - len("/  ") - len("   esc clear")
		m.write(x, l.tabRow, cv.Cols-2, 1, span{"/ ", cSignal}, span{f, cPaper}, span{"   esc clear", cMist})
	}

	for c := 0; c < cv.Cols; c++ {
		cv.Glyph(c, l.ruleRow, '─', cAsh)
	}
	for c := int(math.Round(m.tabX[0])); c < int(math.Round(m.tabX[1])); c++ {
		cv.Glyph(c, l.ruleRow, '━', cSignal)
	}
}

func (m *Model) columns() []placed {
	return place(sectionColumns[m.sec], textCol, m.cv.Cols-textCol-2)
}

// rowProgress is how far a list row has printed in (1 = done).
func (m *Model) rowProgress(id string, vi int) float64 {
	if m.reduce {
		return 1
	}
	b := m.sectionBorn + float64(vi)*printStagger
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
		m.write(x, l.headRow, cv.Cols-2, 1, span{c.title, cMist})
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
		bg := cNight
		if vi == m.hover {
			bg = cRaise
		}
		bg = dots.Lerp(bg, cLift, float32(sel))
		if bg != cNight {
			for c := 0; c < cv.Cols-1; c++ {
				cv.Back(c, y, bg)
			}
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
			m.print(glyphCol, y, string(g), gc, p)
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
		cv.Dot(markCol*2, y0+dy, cSignal, 3)
		cv.Dot(markCol*2+1, y0+dy, cSignal, 3)
	}

	// Scrollbar, only when the list doesn't fit.
	if n := len(rows); n > l.listRows {
		thumb := max(1, l.listRows*l.listRows/n)
		at := off * (l.listRows - thumb) / (n - l.listRows)
		for i := 0; i < l.listRows; i++ {
			g, c := '│', cAsh
			if i >= at && i < at+thumb {
				g, c = '┃', cMist
			}
			cv.Glyph(cv.Cols-1, l.listTop+i, g, c)
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
		m.write(textCol, l.listTop+1+i, m.cv.Cols-2, p, s)
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

// drawDetail is the panel under the list: everything about the selection.
func (m *Model) drawDetail() {
	cv, l := m.cv, m.lay
	for c := 1; c < cv.Cols-1; c += 2 {
		cv.Glyph(c, l.detailRule, '·', cAsh)
	}
	r, ok := m.selected()
	if !ok || m.sec == podsTab {
		return
	}
	p := 1.0
	if !m.reduce {
		p = (m.now - m.detailBorn) / 0.18
	}
	end := cv.Cols - 2
	y := l.detailTop
	label := func(s string) span { return span{s + "  ", cMist} }
	gap := span{"     ", cMist}

	switch {
	case r.c != nil:
		c := r.c
		m.write(glyphCol, y, end, p, span{c.Name, cPaper}, span{"  " + shortID(c.ID), cMist}, span{"  " + c.Image, dots.Lerp(cMist, cPaper, 0.55)})
		health := c.Health
		if health == "" {
			health = "no healthcheck"
		}
		m.write(glyphCol, y+1, end, p,
			label("STATE"), span{string(r.look.Glyph) + " " + r.look.Word, dots.Hex(r.look.Color)}, gap,
			label("HEALTH"), span{health, cPaper})
		m.write(glyphCol, y+2, end, p, label("STATUS"), span{c.Status, cPaper})
		ports := c.Ports
		if ports == "" {
			ports = "none published"
		}
		m.write(glyphCol, y+3, end, p, label("PORTS"), span{ports, cPaper})

	case r.img != nil:
		im := r.img
		name := "<none>"
		if len(im.Tags) > 0 {
			name = im.Tags[0]
		}
		m.write(glyphCol, y, end, p, span{name, cPaper}, span{"  " + im.ShortID, cMist})
		created := "unknown"
		if !im.CreatedAt.IsZero() {
			created = ago(im.CreatedAt, m.wall) + " · " + im.CreatedAt.Format("2006-01-02")
		}
		m.write(glyphCol, y+1, end, p, label("SIZE"), span{humanSize(im.Size), cPaper}, gap, label("CREATED"), span{created, cPaper})
		next := y + 2
		switch {
		case len(im.Tags) > 1:
			m.write(glyphCol, next, end, p, label("ALSO"), span{strings.Join(im.Tags[1:], ", "), cPaper})
			next++
		case len(im.Tags) == 0:
			m.write(glyphCol, next, end, p, label("UNTAGGED"), span{"a dangling image, safe to prune if no container uses it", cMist})
			next++
		}
		if len(im.Digests) > 0 {
			m.write(glyphCol, next, end, p, label("DIGEST"), span{im.Digests[0], cMist})
		}
	}
}

// drawKeys replaces the detail panel with every key binding.
func (m *Model) drawKeys() {
	cv, l := m.cv, m.lay
	for c := 1; c < cv.Cols-1; c += 2 {
		cv.Glyph(c, l.detailRule, '·', cAsh)
	}
	colW := min(30, (cv.Cols-glyphCol-2)/len(m.keys.FullHelp()))
	for ci, group := range m.keys.FullHelp() {
		x := glyphCol + ci*colW
		for ri, b := range group {
			h := b.Help()
			m.write(x, l.detailTop+ri, min(x+colW-1, cv.Cols-2), 1,
				span{fmt.Sprintf("%-5s", h.Key), cSignal}, span{" " + h.Desc, cMist})
		}
	}
}

// drawFooter is the key hints on the left and the sync state on the right.
func (m *Model) drawFooter() {
	cv, l := m.cv, m.lay
	y := l.helpRow

	var right []span
	switch {
	case m.loading > 0:
		g := [...]rune{'·', '•', '●', '•'}[int(m.now*8)%4]
		right = []span{{string(g) + " syncing", cMist}}
	case m.cErr != nil || m.iErr != nil:
		right = []span{{"docker unreachable", cSignal}}
	case !m.synced.IsZero():
		right = []span{{"synced " + m.synced.Format("15:04:05"), cMist}}
	}
	rw := 0
	for _, s := range right {
		rw += len([]rune(s.s))
	}
	limit := cv.Cols - 2 - rw - 2
	m.write(cv.Cols-2-rw, y, cv.Cols-2, 1, right...)

	if m.filtering {
		x := m.write(2, y, limit, 1, span{"/ ", cSignal}, span{m.filter, cPaper})
		if int(m.now*2)%2 == 0 || m.reduce {
			cv.Glyph(x, y, '▏', cSignal)
		}
		m.write(x+3, y, limit, 1, span{"enter", cSignal}, span{" keep   ", cMist}, span{"esc", cSignal}, span{" clear", cMist})
		return
	}
	x := 2
	for _, b := range m.keys.ShortHelp() {
		h := b.Help()
		w := len([]rune(h.Key)) + 1 + len([]rune(h.Desc))
		if x+w > limit {
			break
		}
		x = m.write(x, y, limit, 1, span{h.Key, cSignal}, span{" " + h.Desc, cMist}) + 3
	}
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
