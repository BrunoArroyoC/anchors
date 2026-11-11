package dashboard

import (
	"math"
	"strconv"

	"docker-tool/internal/models"
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

// ship is the halftone container ship beside the box.
type ship struct {
	tone      *dots.Tone
	on        bool
	off, size vec       // image placement, in units (a cell is 1 wide, 2 tall)
	slots     []dotRect // one per cargo box, filled bottom tier first
	block     dotRect   // all of the redrawn tiers, kept clear of halftone
	edges     []bool    // the art's outline, per Braille dot
	legendRow int
	hover     int // slot under the mouse, -1 for none
}

// place fits the ship into a column on the left and returns the first
// column free for the box, or 0 when the terminal is too small for it.
func (s *ship) place(cv *dots.Canvas) int {
	W, H := cv.Cols, cv.Rows
	s.on = s.tone != nil && W >= shipMinCols && H >= 24
	if !s.on {
		return 0
	}
	aspect := float64(s.tone.W) * (cropU1 - cropU0) / (float64(s.tone.H) * (cropV1 - cropV0))
	iw := float64(W) * 0.33
	ih := iw / aspect
	if maxH := float64(H*2 - 6); ih > maxH {
		ih, iw = maxH, maxH*aspect
	}
	s.size = vec{iw, ih}
	s.off = vec{3, (float64(H*2)-ih)/2 - 1}
	s.legendRow = min(H-2, int((s.off.Y+ih)/2)+1)

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
	return int(s.off.X+iw) + 3
}

// at samples the cropped art (0..1 across what is drawn), black outside it.
func (s *ship) at(u, v float64) float64 {
	if u < 0 || v < 0 || u > 1 || v > 1 {
		return 0
	}
	return s.tone.At(cropU0+u*(cropU1-cropU0), cropV0+v*(cropV1-cropV0))
}

// clearing is how far a point (units) is from the ship's patch of calm
// water: everything above the waterline in its column.
func (s *ship) clearing(p vec) float64 {
	dx := math.Max(0, math.Max(s.off.X-p.X, p.X-(s.off.X+s.size.X)))
	dy := math.Max(0, math.Max(s.off.Y-p.Y, p.Y-(s.off.Y+s.size.Y*cropV(waterV))))
	return math.Hypot(dx, dy)
}

// slotAt returns the cargo slot under a terminal cell, or -1.
func (s *ship) slotAt(col, row int) int {
	if !s.on {
		return -1
	}
	x, y := col*2+1, row*4+2
	for k, r := range s.slots {
		if x >= r.x0-1 && x <= r.x1 && y >= r.y0-2 && y <= r.y1+1 {
			return k
		}
	}
	return -1
}

// bayer is a 4x4 ordered threshold map for the stipple.
var bayer = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

// drawShip paints the ship over the sea as a fine stipple at Braille
// resolution (an engraving next to the sea's coarse halftone), its
// outline, and the top tiers as live cargo. The cursor's lens magnifies it
// and its wake and ripples warm it like the sea.
func (m *Model) drawShip() {
	s := &m.ship
	if !s.on {
		return
	}
	cv := m.cv
	dev := 1.0
	if !m.reduce {
		dev = smooth(0, 0.9, m.now)
	}
	R := m.reach()
	y0, y1 := max(0, int(s.off.Y*2)), min(cv.H, int((s.off.Y+s.size.Y)*2)+1)
	x0, x1 := max(0, int(s.off.X*2)), min(cv.W, int((s.off.X+s.size.X)*2)+1)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if s.block.has(x, y) {
				continue
			}
			p := vec{(float64(x) + 0.5) / 2, (float64(y) + 0.5) / 2}
			src, lens := p, 0.0
			if m.lensOn > 0 {
				if d := math.Hypot(p.X-m.lens.X, p.Y-m.lens.Y); d < R {
					lens = (1 - d/R) * (1 - d/R) * m.lensOn
					if !m.reduce {
						src = vec{m.lens.X + (p.X-m.lens.X)*(1-0.4*lens), m.lens.Y + (p.Y-m.lens.Y)*(1-0.4*lens)}
					}
				}
			}
			t := s.at((src.X-s.off.X)/s.size.X, (src.Y-s.off.Y)/s.size.Y)
			v := smooth(0.03, 0.7, t)*dev + lens*0.15
			edge := s.edges[y*cv.W+x]
			if v <= (bayer[y%4][x%4]+0.5)/16 && !edge {
				continue
			}
			cell := y/4*cv.Cols + x/2
			warm := math.Min(1, lens*0.6+m.heat[cell]*1.1+m.wave(p)*0.8)
			c := dots.Lerp(dots.Lerp(cAsh, cMist, 0.6), cPaper, float32(smooth(0.1, 0.9, t)))
			if edge {
				c = dots.Lerp(c, cPaper, 0.3)
			}
			cv.Dot(x, y, dots.Lerp(c, cSignal, float32(warm*0.9)), 1)
		}
	}

	m.drawCargo()
	m.drawLegend()
}

// drawCargo draws one box per container on the top tiers. Running ones are
// solid corrugated steel and breathe while you work; stopped ones are
// hollow; anything that needs a look is signal orange. The selected
// container is ringed. On open the boxes drop onto the deck one by one.
func (m *Model) drawCargo() {
	s, cv := &m.ship, m.cv
	selID := ""
	if m.sec == containersTab {
		if r, ok := m.selected(); ok {
			selID = r.id
		}
	}
	breath := 1 - m.ambient()*(0.5-0.5*math.Cos(m.now*2.4))*0.45

	for k, b := range s.slots {
		var c *models.DockerModel
		if k < len(m.containers) {
			c = &m.containers[k]
		}
		start := boxOpen*0.6 + float64(k)*0.035
		if c != nil {
			if t, ok := m.born[c.ID]; ok && t > start {
				start = t
			}
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

		if c == nil || !m.cLoaded {
			// An empty slot keeps the stack's shape as a faint outline.
			for x := r.x0; x < r.x1; x += 2 {
				cv.Dot(x, r.y0, cAsh, 1)
				cv.Dot(x, r.y1-1, cAsh, 1)
			}
			for y := r.y0; y < r.y1; y += 2 {
				cv.Dot(r.x0, y, cAsh, 1)
				cv.Dot(r.x1-1, y, cAsh, 1)
			}
			continue
		}

		look := theme.ContainerLook(c.State, c.Health)
		col := dots.Hex(look.Color)
		solid := c.State == "running" || c.State == "restarting"
		if look.Word == "running" {
			col = dots.Lerp(cMist, cPaper, float32(breath))
		}
		if !solid && col == cMist {
			col = dots.Lerp(cAsh, cMist, 0.7)
		}
		if t, ok := m.flash[c.ID]; ok && m.now-t < flashDur {
			col = dots.Lerp(col, cSignal, float32(1-(m.now-t)/flashDur))
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
					cv.Dot(x, y, col, 2)
				}
			}
		}

		ring := dots.RGB{}
		switch {
		case c.ID == selID && col == cSignal:
			ring = cPaper // orange on orange would vanish
		case c.ID == selID:
			ring = cSignal
		case k == s.hover:
			ring = cMist
		}
		if ring != (dots.RGB{}) {
			for x := r.x0 - 1; x <= r.x1; x++ {
				cv.Dot(x, r.y0-2, ring, 3)
				cv.Dot(x, r.y1+1, ring, 3)
			}
			for y := r.y0 - 2; y <= r.y1+1; y++ {
				cv.Dot(r.x0-2, y, ring, 3)
				cv.Dot(r.x1+1, y, ring, 3)
			}
		}
	}
}

// drawLegend explains the cargo under the ship.
func (m *Model) drawLegend() {
	s := &m.ship
	if !m.cLoaded || s.legendRow >= m.cv.Rows-1 {
		return
	}
	x, end := int(s.off.X)+1, int(s.off.X+s.size.X)
	for row := s.legendRow; row <= min(s.legendRow+1, m.cv.Rows-2); row++ {
		for c := x - 1; c <= end; c++ {
			m.cv.Glyph(c, row, ' ', cNight) // a clear strip so the sea doesn't run into the words
		}
	}
	parts := []span{{"▥", cPaper}, {" running   ", cMist}, {"▯", cMist}, {" stopped   ", cMist}, {"▥", cSignal}, {" needs a look", cMist}}
	x = m.write(x, s.legendRow, end, 1, parts...)
	if extra := len(m.containers) - len(s.slots); extra > 0 && s.legendRow+1 < m.cv.Rows-1 {
		m.write(int(s.off.X)+1, s.legendRow+1, end, 1, span{"+", cPaper}, span{" " + strconv.Itoa(extra) + " more below deck", cMist})
	}
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
