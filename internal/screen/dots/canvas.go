package dots

import (
	"strconv"
	"strings"
)

// Canvas is a grid of Cols x Rows terminal cells. Each cell shows either a
// Braille pattern (addressed in dots: W = Cols*2, H = Rows*4) or a single
// glyph such as a halftone dot or a letter. A glyph wins over Braille.
type Canvas struct {
	Cols, Rows int
	W, H       int
	bits       []uint8
	col        []RGB
	pri        []uint8
	glyph      []rune
	gcol       []RGB
}

// NewCanvas makes a canvas that fills cols x rows terminal cells.
func NewCanvas(cols, rows int) *Canvas {
	n := cols * rows
	return &Canvas{Cols: cols, Rows: rows, W: cols * 2, H: rows * 4,
		bits: make([]uint8, n), col: make([]RGB, n), pri: make([]uint8, n),
		glyph: make([]rune, n), gcol: make([]RGB, n)}
}

// Clear empties every cell.
func (c *Canvas) Clear() {
	clear(c.bits)
	clear(c.pri)
	clear(c.glyph)
}

// Braille dot bits, indexed [dy][dx] inside a cell.
var dotBit = [4][2]uint8{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// Dot turns on the Braille dot at x,y. pri decides which color a shared cell
// shows; on a tie the brighter color wins.
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

// HasDots reports whether the cell already holds Braille dots.
func (c *Canvas) HasDots(col, row int) bool {
	if col < 0 || row < 0 || col >= c.Cols || row >= c.Rows {
		return false
	}
	return c.bits[row*c.Cols+col] != 0
}

// Glyph puts one character in a cell. A zero rune clears it.
func (c *Canvas) Glyph(col, row int, r rune, fg RGB) {
	if col < 0 || row < 0 || col >= c.Cols || row >= c.Rows {
		return
	}
	i := row*c.Cols + col
	c.glyph[i], c.gcol[i] = r, fg
}

// Text writes s from cell col,row. Spaces are written too, so text always
// reads on a clean ground.
func (c *Canvas) Text(col, row int, s string, fg RGB) {
	for _, r := range s {
		c.Glyph(col, row, r, fg)
		col++
	}
}

// Render returns the frame as ANSI truecolor text on a solid background.
func (c *Canvas) Render(bg RGB) string {
	var b strings.Builder
	b.Grow(c.Cols * c.Rows * 8)
	bgc := bg.u8()
	for r := 0; r < c.Rows; r++ {
		writeSGR(&b, 48, bgc)
		var last [3]uint8
		has := false
		for x := 0; x < c.Cols; x++ {
			i := r*c.Cols + x
			ch, fg := ' ', [3]uint8{}
			switch {
			case c.glyph[i] != 0:
				ch, fg = c.glyph[i], c.gcol[i].u8()
			case c.bits[i] != 0:
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
