package dots

import (
	"bytes"
	"image/png"
	"math"
)

// Tone is a grayscale image (0 black .. 1 white) kept at low resolution and
// slightly blurred, so a halftone made from it has smooth gradients instead
// of the vector art's hard steps.
type Tone struct {
	W, H int
	v    []float32
}

// LoadTone decodes a PNG and reduces it to about width pixels wide.
func LoadTone(data []byte, width int, blur int) (*Tone, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w := min(width, b.Dx())
	h := max(1, int(float64(w)*float64(b.Dy())/float64(b.Dx())))
	sx, sy := float64(b.Dx())/float64(w), float64(b.Dy())/float64(h)
	t := &Tone{W: w, H: h, v: make([]float32, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float64
			var n int
			for yy := int(float64(y) * sy); yy < int(float64(y+1)*sy); yy++ {
				for xx := int(float64(x) * sx); xx < int(float64(x+1)*sx); xx++ {
					r, g, bb, _ := img.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					sum += (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bb)) / 65535
					n++
				}
			}
			if n > 0 {
				t.v[y*w+x] = float32(sum / float64(n))
			}
		}
	}
	for range blur {
		t.boxBlur()
	}
	return t, nil
}

func (t *Tone) boxBlur() {
	out := make([]float32, len(t.v))
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			var s float32
			var n float32
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < t.W && yy < t.H {
						s += t.v[yy*t.W+xx]
						n++
					}
				}
			}
			out[y*t.W+x] = s / n
		}
	}
	t.v = out
}

// At samples the tone at u,v in 0..1 image coordinates (bilinear, clamped).
func (t *Tone) At(u, v float64) float64 {
	x := math.Max(0, math.Min(float64(t.W-1), u*float64(t.W)-0.5))
	y := math.Max(0, math.Min(float64(t.H-1), v*float64(t.H)-0.5))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, t.W-1), min(y0+1, t.H-1)
	fx, fy := float32(x-float64(x0)), float32(y-float64(y0))
	a := t.v[y0*t.W+x0]*(1-fx) + t.v[y0*t.W+x1]*fx
	b := t.v[y1*t.W+x0]*(1-fx) + t.v[y1*t.W+x1]*fx
	return float64(a*(1-fy) + b*fy)
}
