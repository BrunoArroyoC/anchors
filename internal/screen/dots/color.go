// Package dots draws in the terminal with Braille characters: every cell holds
// a 2x4 grid of dots, which gives hand-drawn line art and particles four times
// the resolution of half-block pixels.
package dots

import "strconv"

// RGB is a color with 0..255 float channels so blending stays cheap and exact.
type RGB struct{ R, G, B float32 }

// Hex parses "#rrggbb".
func Hex(s string) RGB {
	if len(s) == 7 && s[0] == '#' {
		v, err := strconv.ParseUint(s[1:], 16, 32)
		if err == nil {
			return RGB{float32(v >> 16 & 0xff), float32(v >> 8 & 0xff), float32(v & 0xff)}
		}
	}
	return RGB{255, 0, 255} // loud magenta: a bad hex is a bug, make it visible
}

// Lerp mixes a toward b by t (0..1).
func Lerp(a, b RGB, t float32) RGB {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return RGB{a.R + (b.R-a.R)*t, a.G + (b.G-a.G)*t, a.B + (b.B-a.B)*t}
}

// Add returns a + b*k, clamped (additive light).
func Add(a, b RGB, k float32) RGB {
	return RGB{clamp(a.R + b.R*k), clamp(a.G + b.G*k), clamp(a.B + b.B*k)}
}

// Luma is perceived brightness, 0..1.
func (c RGB) Luma() float32 { return (0.299*c.R + 0.587*c.G + 0.114*c.B) / 255 }

func (c RGB) u8() [3]uint8 { return [3]uint8{uint8(clamp(c.R)), uint8(clamp(c.G)), uint8(clamp(c.B))} }

func clamp(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
