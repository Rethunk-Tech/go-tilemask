package tilemask

import (
	"bytes"
	"testing"
)

// When the fast path and image/png both accept an input their masks match. An input only the fast path accepts is
// allowed (it skips checksums), and one it refuses goes to image/png by design.
func FuzzDecodePNG(f *testing.F) {
	for color := range byte(7) {
		for filter := range byte(5) {
			spec := pngSpec{w: 5, h: 4, color: color, filter: filter}
			spec.px = pseudoBytes(5*4*max(pngChannels(color), 1), uint32(color)+uint32(filter))
			if color == 3 {
				spec.palette = []byte{0, 0, 0, 255, 0, 0}
				spec.trns = []byte{0, 255}
				for i := range spec.px {
					spec.px[i] %= 3
				}
			}
			if color == 0 || color == 2 {
				spec.trns = []byte{0, 2, 0, 3, 0, 4}[:2+4*int(color/2)]
			}
			if pngChannels(color) != 0 {
				f.Add(buildPNG(f, spec))
			}
		}
	}
	for _, d := range [][2]uint32{{0x7fffffff, 0x7fffffff}, {0xffffffff, 0xffffffff}, {1, 0xffffffff}, {uint32(DefaultMaxPixels + 1), 1}} {
		f.Add(append([]byte(pngSignature), pngChunk("IHDR", ihdr(d[0], d[1], 8, 6, 0))...))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		const limit = 1 << 20
		fast, ferr := fastPNG(raw, limit)
		std, serr := stdPNG(raw, limit)
		if ferr == nil && serr == nil && !masksEqual(fast, std) {
			t.Fatalf("fast path and image/png disagree on %x", raw)
		}
		if m, err := DecodePNG(bytes.NewReader(raw), Options{MaxPixels: limit}); err == nil && int64(m.Width)*int64(m.Height) > limit {
			t.Fatalf("mask %d x %d exceeds the cap", m.Width, m.Height)
		}
	})
}
