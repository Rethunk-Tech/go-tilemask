package tilemask

import (
	"image"
	"iter"
	"math"
)

// Mask is one bit per pixel, set when the pixel's alpha is not zero. It is ordinary garbage-collected memory and
// safe for concurrent reads.
type Mask struct {
	Width, Height int
	// bits is row-major and least-significant-bit first: pixel i = y*Width+x is bit i&7 of bits[i>>3]. Bits past
	// the last pixel stay zero, so equal masks hold equal bytes.
	bits []byte
}

func newMask(w, h int) *Mask {
	return &Mask{Width: w, Height: h, bits: make([]byte, (w*h+7)/8)}
}

func (m *Mask) set(i int) { m.bits[i>>3] |= 1 << (i & 7) }

// setAll marks every pixel, leaving the padding bits clear.
func (m *Mask) setAll() {
	for i := range m.bits {
		m.bits[i] = 0xff
	}
	if rem := m.Width * m.Height & 7; rem != 0 {
		m.bits[len(m.bits)-1] = 1<<rem - 1
	}
}

// At reports whether the pixel is set. It is false outside the bounds.
func (m *Mask) At(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return false
	}
	i := y*m.Width + x
	return m.bits[i>>3]&(1<<(i&7)) != 0
}

// maxCoord bounds the coordinates Cells accepts, so no sum of them with a mask dimension can wrap.
const maxCoord = math.MaxInt / 4

// Cells yields each cell that holds a set pixel of src, row-major and each once. A cell is cell by cell pixels, src
// is clipped to the mask, and src's top-left lands at dst. Cell coordinates use floor division, so pixels left of
// or above the origin fall in cell -1, not cell 0. It yields nothing when cell <= 0, when src is empty after
// clipping, or when a coordinate of src or dst is beyond a quarter of the int range.
//
// Cells holds one bool per cell of the covered range while it runs, so cell = 1 costs a byte per pixel.
func (m *Mask) Cells(src image.Rectangle, dst image.Point, cell int) iter.Seq[image.Point] {
	return func(yield func(image.Point) bool) {
		src := src
		if cell <= 0 || !coordOK(dst) || !coordOK(src.Min) {
			return
		}
		ox, oy := dst.X-src.Min.X, dst.Y-src.Min.Y
		src = src.Intersect(image.Rect(0, 0, m.Width, m.Height))
		if src.Empty() {
			return
		}
		firstX, lastX := floorDiv(ox+src.Min.X, cell), floorDiv(ox+src.Max.X-1, cell)
		firstY, lastY := floorDiv(oy+src.Min.Y, cell), floorDiv(oy+src.Max.Y-1, cell)
		width := lastX - firstX + 1
		hit := make([]bool, width*(lastY-firstY+1))
		for y := src.Min.Y; y < src.Max.Y; y++ {
			cy := floorDiv(oy+y, cell) - firstY
			for x := src.Min.X; x < src.Max.X; x++ {
				if m.At(x, y) {
					hit[cy*width+floorDiv(ox+x, cell)-firstX] = true
				}
			}
		}
		for i, set := range hit {
			if set && !yield(image.Pt(firstX+i%width, firstY+i/width)) {
				return
			}
		}
	}
}

func coordOK(p image.Point) bool {
	return p.X >= -maxCoord && p.X <= maxCoord && p.Y >= -maxCoord && p.Y <= maxCoord
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
