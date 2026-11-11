package dashboard

import (
	"math"

	"docker-tool/internal/screen/dots"
	"docker-tool/internal/screen/theme"
)

// The scene behind the logbook: a night sky with stars, a halftone sea in
// perspective below the horizon, and a lighthouse on the rocks to the right
// whose beam points at the container you have selected.

// The lighthouse art, in 0..1 image coordinates (measured on the 1000x1750
// render): the part that is drawn, the lantern, and the foot of the rocks.
const (
	lhCropU0, lhCropU1 = 0.12, 0.88
	lhCropV0, lhCropV1 = 0.07, 0.95
	lanternU, lanternV = 0.5, 0.166
	rocksV             = 0.93 // where the rocks meet the sea: the horizon
	lighthouseMinCols  = 100  // narrower terminals get the logbook alone

	beamWidth = 0.055 // half-width of the beam at the lamp, in radians
	horizonAt = 0.70  // the horizon, as a share of the screen's height
)

// bayer is a 4x4 ordered threshold map for stippling.
var bayer = [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}

type star struct {
	x, y   int
	bright float64
	phase  float64
}

// scene is everything behind the logbook.
type scene struct {
	tone      *dots.Tone // the lighthouse art
	on        bool       // the lighthouse fits
	off, size vec        // lighthouse placement, in units (a cell is 1 wide, 2 tall)
	lamp      vec        // the lantern, in units
	horizon   int        // first row of sea
	mask      []bool     // cells the lighthouse covers, kept clear of sky and sea
	stars     []star
}

func lhU(u float64) float64 { return (u - lhCropU0) / (lhCropU1 - lhCropU0) }
func lhV(v float64) float64 { return (v - lhCropV0) / (lhCropV1 - lhCropV0) }

// at samples the cropped lighthouse art, black outside it.
func (s *scene) at(u, v float64) float64 {
	if u < 0 || v < 0 || u > 1 || v > 1 {
		return 0
	}
	return s.tone.At(lhCropU0+u*(lhCropU1-lhCropU0), lhCropV0+v*(lhCropV1-lhCropV0))
}

// place stands the lighthouse on the right and returns the last column the
// logbook may use, or 0 when the terminal is too small for the lighthouse.
func (s *scene) place(cv *dots.Canvas) int {
	W, H := cv.Cols, cv.Rows
	s.on = s.tone != nil && W >= lighthouseMinCols && H >= 24
	s.horizon = int(float64(H) * horizonAt)
	right := 0
	if s.on {
		// The lighthouse stands on the horizon, its rocks breaking it, and
		// reaches most of the way up the sky.
		aspect := float64(s.tone.W) * (lhCropU1 - lhCropU0) / (float64(s.tone.H) * (lhCropV1 - lhCropV0))
		ih := float64(H*2) * 0.64
		iw := ih * aspect
		s.size = vec{iw, ih}
		s.off = vec{float64(W) - iw - float64(W)*0.06, float64(s.horizon*2) + 1 - lhV(rocksV)*ih}
		s.lamp = vec{s.off.X + lhU(lanternU)*iw, s.off.Y + lhV(lanternV)*ih}
		gap := max(10, int(float64(W)*0.1)) // open water for the beam to cross
		right = int(s.off.X) - gap

		s.mask = make([]bool, W*H)
		for row := 0; row < H; row++ {
			for col := 0; col < W; col++ {
				for _, d := range [][2]float64{{0.25, 0.5}, {0.75, 0.5}, {0.25, 1.5}, {0.75, 1.5}} {
					u := (float64(col) + d[0] - s.off.X) / iw
					v := (float64(row)*2 + d[1] - s.off.Y) / ih
					if s.at(u, v) > 0.05 {
						s.mask[row*W+col] = true
						break
					}
				}
			}
		}
	}

	// Stars: a few hundred fixed points in the sky, at Braille resolution.
	s.stars = s.stars[:0]
	for y := 0; y < s.horizon*4; y++ {
		for x := 0; x < cv.W; x++ {
			if h := hash(x*7+3, y*13+1); h < 0.0045 {
				s.stars = append(s.stars, star{x, y, 0.35 + 0.65*hash(x, y+999), h * 1400})
			}
		}
	}
	return right
}

// beamTarget is where the beam should point: the logbook's right edge, on
// the selected row (or the first line of the list when nothing is there).
func (m *Model) beamTarget() vec {
	l := m.lay
	row := l.listTop + 1
	if _, ok := m.selected(); ok && m.sec != podsTab {
		row = l.listTop + m.sel[m.sec] - m.off[m.sec]
	}
	return vec{float64(l.bx1) + 0.5, float64(row)*2 + 1}
}

// beamAngle is the angle the beam should settle on.
func (m *Model) beamAngle() float64 {
	t := m.beamTarget()
	return math.Atan2(t.Y-m.scene.lamp.Y, t.X-m.scene.lamp.X)
}

// beamColor is paper-white, or signal orange when the selected container
// needs a look.
func (m *Model) beamColor() dots.RGB {
	if r, ok := m.selected(); ok && r.c != nil && dots.Hex(r.look.Color) == cSignal {
		return dots.Lerp(cPaper, cSignal, 0.85)
	}
	return dots.Lerp(cPaper, dots.Hex(theme.Signal), 0.12) // warm lamplight
}

// beamAt is how much of the beam reaches a point (units), 0..1.
func (m *Model) beamAt(p vec) float64 {
	s := &m.scene
	if !s.on {
		return 0
	}
	if p.X < float64(m.lay.bx1)+1 {
		return 0 // the logbook stops the light
	}
	dx, dy := p.X-s.lamp.X, p.Y-s.lamp.Y
	dist := math.Hypot(dx, dy)
	halo := math.Max(0, 1-dist/6) * 0.7 // the lantern glows all round
	if dist < 1 {
		return 1
	}
	a := m.beam + m.sway()
	d := math.Abs(math.Remainder(math.Atan2(dy, dx)-a, 2*math.Pi))
	// The beam spreads a little as it travels, so it lands as a pool of
	// light rather than a needle.
	w := beamWidth * (1 + dist/float64(m.cv.Cols)*1.5)
	if d > w {
		return halo
	}
	core := 1 - smooth(w*0.3, w, d)
	return math.Max(halo, core*(0.55+0.45*math.Exp(-dist/float64(m.cv.Cols))))
}

// sway is the beam's slow drift while you work, plus the sweep after a
// refresh.
func (m *Model) sway() float64 {
	if m.reduce {
		return 0
	}
	a := m.ambient() * 0.012 * math.Sin(m.now*0.9)
	if k := (m.now - m.sweepAt) / 0.9; m.sweepAt > 0 && k >= 0 && k < 1 {
		a -= 0.55 * math.Sin(math.Pi*k) * math.Sin(math.Pi*k)
	}
	return a
}

// drawScene paints sky, sea, lighthouse and beam around the open part of
// the logbook (rows top..bot) and the footer's span.
func (m *Model) drawScene(top, bot int, hints [2]int) {
	cv, l, s := m.cv, m.lay, &m.scene
	dev := 1.0
	if !m.reduce {
		dev = smooth(0, 0.6, m.now)
	}
	free := func(col, row int) bool {
		if col >= l.bx0-1 && col <= l.bx1+1 && row >= top && row <= bot {
			return false
		}
		if row == l.helpRow && col >= hints[0]-2 && col < hints[1]+2 {
			return false
		}
		return !(s.on && s.mask[row*cv.Cols+col])
	}
	beamCol := m.beamColor()
	amb := m.ambient()

	// Sky: stars, and the beam as a shaft of haze.
	for _, st := range s.stars {
		col, row := st.x/2, st.y/4
		if !free(col, row) {
			continue
		}
		b := st.bright
		if !m.reduce {
			b *= 1 - amb*0.5*(0.5+0.5*math.Sin(m.now*2.6+st.phase))
		}
		cv.Dot(st.x, st.y, dots.Lerp(cNight, dots.Lerp(cMist, cPaper, 0.4), float32(b*dev)), 1)
	}
	if s.on {
		for row := 0; row < s.horizon; row++ {
			for col := 0; col < cv.Cols; col++ {
				if !free(col, row) {
					continue
				}
				b := m.beamAt(vec{float64(col) + 0.5, float64(row)*2 + 1}) * dev
				if b < 0.02 {
					continue
				}
				for dy := 0; dy < 4; dy++ {
					for dx := 0; dx < 2; dx++ {
						x, y := col*2+dx, row*4+dy
						if b*0.85 > (bayer[y%4][x%4]+0.5)/16 {
							cv.Dot(x, y, dots.Lerp(cNight, beamCol, float32(0.3+0.5*b)), 1)
						}
					}
				}
			}
		}
	}

	// Sea: halftone in perspective, larger and denser toward you, with
	// swells rolling in while you work. The beam lays a path of light on it.
	R := m.reach()
	H := float64(cv.Rows - s.horizon)
	for row := s.horizon; row < cv.Rows; row++ {
		depth := float64(row-s.horizon+1) / H // 0 at the horizon, 1 at your feet
		for col := row % 2; col < cv.Cols; col += 2 {
			if !free(col, row) {
				continue
			}
			p := vec{float64(col) + 0.5, float64(row)*2 + 1}
			src, lens := p, 0.0
			if m.lensOn > 0 {
				if d := math.Hypot(p.X-m.lens.X, p.Y-m.lens.Y); d < R {
					lens = (1 - d/R) * (1 - d/R) * m.lensOn
					if !m.reduce {
						src = vec{m.lens.X + (p.X-m.lens.X)*(1-0.45*lens), p.Y}
					}
				}
			}
			t := m.swellT
			swell := 0.5 + 0.5*math.Sin(src.X*0.09*(1.4-depth)+math.Sin(src.X*0.021+t*0.4)*1.6+
				math.Log(depth+0.05)*9-t*1.3)
			v := (0.1 + 0.4*depth) * (0.45 + 0.85*swell) * dev
			b := m.beamAt(p)
			if s.on {
				// The lantern's light on the water: a column that widens
				// toward you and breaks up with the swell.
				spread := 1.2 + depth*7
				refl := math.Exp(-math.Pow((p.X-s.lamp.X)/spread, 2)) * (0.25 + 0.75*swell) * (1 - 0.5*depth)
				b = math.Max(b, refl*0.75*dev)
			}
			heat := m.heat[row*cv.Cols+col]
			wave := m.wave(p)
			v += b*0.7 + lens*0.2 + heat*0.55 + wave*0.6
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
			c := dots.Lerp(dots.Lerp(cAsh, cMist, 0.3), dots.Lerp(cMist, cPaper, 0.35), float32(smooth(0.1, 0.9, v)))
			c = dots.Lerp(c, beamCol, float32(math.Min(1, b*1.2)))
			c = dots.Lerp(c, cSignal, float32(math.Min(1, lens*0.7+heat*1.2+wave)*0.9))
			cv.Glyph(col, row, g, c)
		}
	}

	if s.on {
		m.drawLighthouse(dev, beamCol)
		// Where the beam lands, the logbook's edge catches the light.
		if t := m.beamTarget(); l.bx1 >= 0 {
			row := int(t.Y / 2)
			for d := -1; d <= 1; d++ {
				if r := row + d; r > top && r < bot {
					k := float32(0.9)
					if d != 0 {
						k = 0.4
					}
					cv.Glyph(l.bx1, r, '│', dots.Lerp(cFrame, beamCol, k*float32(dev)))
				}
			}
		}
	}
}

// drawLighthouse stipples the lighthouse at Braille resolution, with the
// lantern burning in the beam's colour.
func (m *Model) drawLighthouse(dev float64, beamCol dots.RGB) {
	cv, s := m.cv, &m.scene
	y0, y1 := max(0, int(s.off.Y*2)), min(cv.H, int((s.off.Y+s.size.Y)*2)+1)
	x0, x1 := max(0, int(s.off.X*2)), min(cv.W, int((s.off.X+s.size.X)*2)+1)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			u := ((float64(x)+0.5)/2 - s.off.X) / s.size.X
			v := ((float64(y)+0.5)/2 - s.off.Y) / s.size.Y
			t := s.at(u, v)
			if smooth(0.03, 0.32, t)*dev <= (bayer[y%4][x%4]+0.5)/16 {
				continue
			}
			c := dots.Lerp(dots.Lerp(cAsh, cMist, 0.5), cPaper, float32(smooth(0.1, 0.9, t)))
			// The lantern takes the beam's colour.
			if d := math.Hypot((float64(x)/2-s.lamp.X)*1.6, float64(y)/2-s.lamp.Y); d < 4 && t > 0.6 {
				c = beamCol
			}
			cv.Dot(x, y, c, 2)
		}
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
