package dashboard

import (
	"math"
	"strconv"

	"docker-tool/internal/screen/dots"
	"docker-tool/internal/screen/theme"
)

// The container stack in the ship art, in 0..1 image coordinates (measured
// on the 1000x1357 render): five tiers, six wide, with four on the top
// tier. Every box is redrawn in code, one per Docker container.
const (
	gridU0, gridPitchU = 0.305, 0.0658
	gridV0, gridPitchV = 0.1201, 0.0556
	gridCols, gridRows = 6, 5
	waterV             = 0.735 // the waterline
	shipMinCols        = 100   // narrower terminals get the box alone

	// The part of the art that is drawn: the ship and a bit of its
	// reflection, without the empty black margins.
	cropU0, cropU1 = 0.08, 0.92
	cropV0, cropV1 = 0.09, 0.86
)

// cropU and cropV turn art coordinates into drawn (cropped) ones.
func cropU(u float64) float64 { return (u - cropU0) / (cropU1 - cropU0) }
func cropV(v float64) float64 { return (v - cropV0) / (cropV1 - cropV0) }

// dotRect is a rectangle in Braille dots, [x0,x1) x [y0,y1).
type dotRect struct{ x0, y0, x1, y1 int }

func (r dotRect) has(x, y int) bool { return x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1 }

// ship is the container ship behind the box, showing through the panel as
// a faint watermark; its cargo boxes are your containers.
type ship struct {
	tone      *dots.Tone
	on        bool
	off, size vec       // image placement, in units (a cell is 1 wide, 2 tall)
	slots     []dotRect // one per cargo box, filled bottom tier first
	block     dotRect   // all of the redrawn tiers, kept clear of the stipple
	edges     []bool    // the art's outline, per Braille dot
}

// place centres the ship in the box, as tall as the box, so it sits behind
// the manifest as a watermark.
func (s *ship) place(cv *dots.Canvas, l layout) {
	s.on = s.tone != nil
	if !s.on {
		return
	}
	aspect := float64(s.tone.W) * (cropU1 - cropU0) / (float64(s.tone.H) * (cropV1 - cropV0))
	ih := float64((l.by1-l.by0+1)*2 - 4)
	iw := ih * aspect
	s.size = vec{iw, ih}
	s.off = vec{float64(l.bx0+l.bx1+1)/2 - iw/2, float64(l.by0*2) + 2}
	iw, ih = s.size.X, s.size.Y

	// Cargo slots: bottom tier first, left to right, so the stack fills up
	// from the deck; the top tier only has the middle four. A one-dot gap
	// separates neighbours.
	dx := func(u float64) int { return int(math.Round((s.off.X + cropU(u)*iw) * 2)) }
	dy := func(v float64) int { return int(math.Round((s.off.Y + cropV(v)*ih) * 2)) }
	s.slots = s.slots[:0]
	for r := gridRows - 1; r >= 0; r-- {
		for c := 0; c < gridCols; c++ {
			if r == 0 && (c == 0 || c == gridCols-1) {
				continue
			}
			u, v := gridU0+float64(c)*gridPitchU, gridV0+float64(r)*gridPitchV
			s.slots = append(s.slots, dotRect{dx(u), dy(v), dx(u+gridPitchU) - 1, dy(v+gridPitchV) - 1})
		}
	}
	s.block = dotRect{dx(gridU0) - 2, dy(gridV0) - 2, dx(gridU0+gridCols*gridPitchU) + 1, dy(gridV0+gridRows*gridPitchV) + 1}

	// Trace the art's edges at dot resolution; they go in the cells between
	// halftone dots so the silhouette stays sharp.
	s.edges = make([]bool, cv.W*cv.H)
	tone := func(x, y float64) float64 { return s.at((x/2-s.off.X)/iw, (y/2-s.off.Y)/ih) }
	for y := max(0, dy(cropV0)); y < min(cv.H, dy(cropV1)); y++ {
		for x := max(0, dx(cropU0)); x < min(cv.W, dx(cropU1)); x++ {
			if s.block.has(x, y) {
				continue
			}
			fx, fy := float64(x)+0.5, float64(y)+0.5
			gx := tone(fx+1.2, fy) - tone(fx-1.2, fy)
			gy := tone(fx, fy+1.2) - tone(fx, fy-1.2)
			s.edges[y*cv.W+x] = math.Hypot(gx, gy) > 0.22
		}
	}
}

// at samples the cropped art (0..1 across what is drawn), black outside it.
func (s *ship) at(u, v float64) float64 {
	if u < 0 || v < 0 || u > 1 || v > 1 {
		return 0
	}
	return s.tone.At(cropU0+u*(cropU1-cropU0), cropV0+v*(cropV1-cropV0))
}

// bayer is a 4x4 ordered threshold map for the stipple.
var bayer = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

// drawShip paints the ship behind the box as a watermark: a fine stipple
// at Braille resolution, faint against the panel, in empty cells only, and
// its cargo, one box per container.
func (m *Model) drawShip(top, bot int) {
	s := &m.ship
	if !s.on {
		return
	}
	cv := m.cv
	dev := 1.0
	if !m.reduce {
		dev = smooth(0, 0.9, m.now)
	}
	y0, y1 := max(0, int(s.off.Y*2)), min(cv.H, int((s.off.Y+s.size.Y)*2)+1)
	x0, x1 := max(0, int(s.off.X*2)), min(cv.W, int((s.off.X+s.size.X)*2)+1)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if s.block.has(x, y) || !m.behind(x/2, y/4, top, bot) || !m.quiet(x/2, y/4) {
				continue
			}
			t := s.at(((float64(x)+0.5)/2-s.off.X)/s.size.X, ((float64(y)+0.5)/2-s.off.Y)/s.size.Y)
			edge := s.edges[y*cv.W+x]
			if smooth(0.03, 0.7, t)*dev <= (bayer[y%4][x%4]+0.5)/16 && !edge {
				continue
			}
			c := dots.Lerp(dots.Lerp(cAsh, cMist, 0.6), cPaper, float32(smooth(0.1, 0.9, t)))
			if edge {
				c = dots.Lerp(c, cPaper, 0.3)
			}
			cv.Dot(x, y, dots.Lerp(cPanel, c, float32(0.17*dev)), 1)
		}
	}
	m.drawCargo(top, bot)
}

// quiet reports whether the watermark may use a cell: it and its
// neighbours on the row hold no text, so words always keep clear space.
func (m *Model) quiet(col, row int) bool {
	cv := m.cv
	return !cv.HasGlyph(col-1, row) && !cv.HasGlyph(col, row) && !cv.HasGlyph(col+1, row)
}

// behind reports whether a cell is under the open part of the box.
func (m *Model) behind(col, row, top, bot int) bool {
	l := m.lay
	return col >= l.bx0 && col <= l.bx1 && row >= top && row <= bot
}

// drawCargo draws the ship's containers, one per Docker container, as part
// of the watermark. Running ones are dense corrugated steel and breathe
// while you work; stopped ones have sparse ribs; anything that needs a look
// is signal orange. The selected container's box glows through the panel
// with a ring around it. On open the boxes drop onto the deck one by one.
func (m *Model) drawCargo(top, bot int) {
	s, cv := &m.ship, m.cv
	selID := ""
	if m.sec == containersTab {
		if r, ok := m.selected(); ok {
			selID = r.id
		}
	}
	breath := 1 - m.ambient()*(0.5-0.5*math.Cos(m.now*2.4))*0.45
	dot := func(x, y int, c dots.RGB, k float32) {
		if m.behind(x/2, y/4, top, bot) && m.quiet(x/2, y/4) {
			cv.Dot(x, y, dots.Lerp(cPanel, c, k), 1)
		}
	}

	for k, b := range s.slots {
		if k >= len(m.containers) || !m.cLoaded {
			break // empty slots stay dark: the stack shows how full you are
		}
		c := &m.containers[k]
		start := boxOpen*0.6 + float64(k)*0.035
		if t, ok := m.born[c.ID]; ok && t > start {
			start = t
		}
		p := 1.0
		if !m.reduce {
			p = math.Min(1, (m.now-start)/0.35)
		}
		if p <= 0 {
			continue
		}
		drop := int(math.Round(-math.Pow(1-p, 3) * 18))
		r := dotRect{b.x0, b.y0 + drop, b.x1, b.y1 + drop}

		look := theme.ContainerLook(c.State, c.Health)
		col := dots.Hex(look.Color)
		solid := c.State == "running" || c.State == "restarting"
		if look.Word == "running" {
			col = dots.Lerp(cMist, cPaper, float32(breath))
		}
		if t, ok := m.flash[c.ID]; ok && m.now-t < flashDur {
			col = dots.Lerp(col, cSignal, float32(1-(m.now-t)/flashDur))
		}
		strength := float32(0.3)
		if c.ID == selID {
			strength = 0.8
		}
		// Corrugated steel: a rib every other dot when it runs, every fourth
		// when it doesn't, so state reads without color too.
		rib := 4
		if solid {
			rib = 2
		}
		for y := r.y0; y < r.y1; y++ {
			for x := r.x0; x < r.x1; x++ {
				edge := y == r.y0 || y == r.y1-1 || x == r.x0 || x == r.x1-1
				if edge || (x-r.x0)%rib == 0 {
					dot(x, y, col, strength)
				}
			}
		}
		if c.ID == selID {
			ring := cSignal
			if col == cSignal {
				ring = cPaper // orange on orange would vanish
			}
			for x := r.x0 - 1; x <= r.x1; x++ {
				dot(x, r.y0-2, ring, 0.7)
				dot(x, r.y1+1, ring, 0.7)
			}
			for y := r.y0 - 2; y <= r.y1+1; y++ {
				dot(r.x0-2, y, ring, 0.7)
				dot(r.x1+1, y, ring, 0.7)
			}
		}
	}
}

// legend explains the cargo; it is set into the box's bottom edge.
func (m *Model) legend() []span {
	if !m.ship.on || !m.cLoaded {
		return nil
	}
	out := []span{{" ▥", cPaper}, {" running  ", cMist}, {"▯", cMist}, {" stopped  ", cMist}, {"▥", cSignal}, {" needs a look ", cMist}}
	if extra := len(m.containers) - len(m.ship.slots); extra > 0 {
		out = append(out, span{"· ", cAsh}, span{strconv.Itoa(extra), cPaper}, span{" below deck ", cMist})
	}
	return out
}

// wave is how much the ripples lift a point.
func (m *Model) wave(p vec) float64 {
	w := 0.0
	for _, r := range m.ripples {
		d := math.Hypot(p.X-r.at.X, p.Y-r.at.Y)
		w += math.Exp(-math.Pow((d-r.age*70)/3, 2)) * (1 - r.age/1.8)
	}
	return w
}
