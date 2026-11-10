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
