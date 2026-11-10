package dots

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"
)

// Art is ink-on-paper artwork ready to be sampled into dots.
type Art struct {
	img    *image.NRGBA
	Bounds image.Rectangle // tight box around the ink
}

// LoadArt decodes a black-ink PNG and finds the box around its ink.
func LoadArt(data []byte) (*Art, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	img := image.NewNRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	a := &Art{img: img}
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if a.ink(x, y) > 0.35 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	a.Bounds = image.Rect(minX, minY, maxX+1, maxY+1)
	return a, nil
}

// Aspect is width / height of the inked area.
func (a *Art) Aspect() float64 { return float64(a.Bounds.Dx()) / float64(a.Bounds.Dy()) }

// ink is how much ink covers a source pixel, 0..1.
func (a *Art) ink(x, y int) float64 {
	c := a.img.NRGBAAt(x, y)
	luma := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
	return (1 - luma) * float64(c.A) / 255
}

// Mask samples the inked area into a w x h grid of dots. A dot is on when
// enough ink falls in its box; the threshold is relative to the box so thin
// pen lines survive heavy downscaling.
func (a *Art) Mask(w, h int, threshold float64) []bool {
	out := make([]bool, w*h)
	r := a.Bounds
	fx := float64(r.Dx()) / float64(w)
	fy := float64(r.Dy()) / float64(h)
	for y := 0; y < h; y++ {
		y0 := r.Min.Y + int(float64(y)*fy)
		y1 := max(y0+1, r.Min.Y+int(math.Ceil(float64(y+1)*fy)))
		for x := 0; x < w; x++ {
			x0 := r.Min.X + int(float64(x)*fx)
			x1 := max(x0+1, r.Min.X+int(math.Ceil(float64(x+1)*fx)))
			var sum, n float64
			for sy := y0; sy < y1 && sy < r.Max.Y; sy++ {
				for sx := x0; sx < x1 && sx < r.Max.X; sx++ {
					sum += a.ink(sx, sy)
					n++
				}
			}
			out[y*w+x] = n > 0 && sum/n >= threshold
		}
	}
	return out
}
