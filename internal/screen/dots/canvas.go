package dots

import (
	"strconv"
	"strings"
)

// Canvas is a grid of Cols x Rows cells, addressed in dots: W = Cols*2,
// H = Rows*4. Each cell has one foreground color; when several layers put
// dots in the same cell, the higher priority layer picks the color.
type Canvas struct {
	Cols, Rows int
	W, H       int
	bits       []uint8
	col        []RGB
	pri        []uint8
}

// NewCanvas makes a canvas that fills cols x rows terminal cells.
func NewCanvas(cols, rows int) *Canvas {
	n := cols * rows
	return &Canvas{Cols: cols, Rows: rows, W: cols * 2, H: rows * 4,
		bits: make([]uint8, n), col: make([]RGB, n), pri: make([]uint8, n)}
}

// Clear removes every dot.
func (c *Canvas) Clear() {
	clear(c.bits)
	clear(c.pri)
}

// Braille dot bits, indexed [dy][dx] inside a cell.
var dotBit = [4][2]uint8{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// Dot turns on the dot at x,y. pri decides which color a shared cell shows;
// on a tie the brighter color wins so lit particles read through.
func (c *Canvas) Dot(x, y int, col RGB, pri uint8) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	i := (y/4)*c.Cols + x/2
	c.bits[i] |= dotBit[y%4][x%2]
	if pri > c.pri[i] || (pri == c.pri[i] && col.Luma() > c.col[i].Luma()) || c.bits[i] == dotBit[y%4][x%2] {
		c.col[i], c.pri[i] = col, pri
	}
}

// Glyph is a text character placed in a cell, over the dots.
type Glyph struct {
	R  rune
	FG RGB
}

// Overlay holds text by cell.
type Overlay struct{ cells map[int]Glyph }

// Put writes s from cell col,row in color fg. Spaces leave the dots visible.
func (o *Overlay) Put(col, row int, s string, fg RGB) {
	if o.cells == nil {
		o.cells = map[int]Glyph{}
	}
	for _, r := range s {
		if r != ' ' {
			o.cells[row<<16|col] = Glyph{r, fg}
		}
		col++
	}
}

// Render returns the frame as ANSI truecolor text. bg gives each row's
// background so the night can deepen toward the bottom.
func (c *Canvas) Render(bg func(row int) RGB, ov *Overlay) string {
	var b strings.Builder
	b.Grow(c.Cols * c.Rows * 8)
	for r := 0; r < c.Rows; r++ {
		writeSGR(&b, 48, bg(r).u8())
		var last [3]uint8
		has := false
		for x := 0; x < c.Cols; x++ {
			i := r*c.Cols + x
			ch, fg := ' ', [3]uint8{}
			if g, ok := ov.cells[r<<16|x]; ok {
				ch, fg = g.R, g.FG.u8()
			} else if c.bits[i] != 0 {
				ch, fg = rune(0x2800+int(c.bits[i])), c.col[i].u8()
			}
			if ch != ' ' && (!has || fg != last) {
				writeSGR(&b, 38, fg)
				last, has = fg, true
			}
			b.WriteRune(ch)
		}
		b.WriteString("\x1b[0m")
		if r < c.Rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func writeSGR(b *strings.Builder, kind int, c [3]uint8) {
	b.WriteString("\x1b[")
	b.WriteString(strconv.Itoa(kind))
	b.WriteString(";2;")
	b.WriteString(strconv.Itoa(int(c[0])))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(c[1])))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(c[2])))
	b.WriteByte('m')
}
