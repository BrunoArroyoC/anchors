// Package assets embeds the art used by the screens.
//
// The PNGs are rasterized (1000px wide) from the Recraft vector sources in
// src/.
package assets

import _ "embed"

// HeroPNG is a large ship anchor on its chain, smooth grayscale lit from the
// upper left on pure black, sitting in the right half of a 16:9 frame. The
// left side is left dark for typography. It is drawn as a halftone.
//
//go:embed hero.png
var HeroPNG []byte

// TitlePNG is the "ANCHORS" wordmark, a bold condensed grotesk, black on white.
//
//go:embed title.png
var TitlePNG []byte

// LighthousePNG is a tall lighthouse on rocks, seen from the front, rim-lit
// on both sides with a glowing lantern, on pure black (1000x1750). The
// dashboard stipples it on the right of the screen and sends a beam from
// its lantern.
//
//go:embed lighthouse.png
var LighthousePNG []byte
