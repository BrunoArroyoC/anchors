// Package assets embeds the art used by the screens.
//
// The PNGs are rasterized (1000px wide) from the Recraft vector sources in
// src/. They are black ink on white; the dots package turns the ink into
// Braille dots at the terminal's size.
package assets

import _ "embed"

// ShipPNG is a hand-drawn ink sketch of a tall ship with its anchor down.
//
//go:embed ship.png
var ShipPNG []byte

// TitlePNG is the handwritten "anchors" wordmark.
//
//go:embed title.png
var TitlePNG []byte
