package tilemask

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
