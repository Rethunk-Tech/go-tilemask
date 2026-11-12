package tilemask

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"runtime"
	"strings"
	"testing"
)

// allocatedBy runs fn and returns the bytes the process allocated meanwhile. The tests using it are not parallel,
// so no other test allocates alongside.
func allocatedBy(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

const hostileAllocLimit = 4 << 20

func TestReadMaskHeaderWithoutNewlineIsBounded(t *testing.T) {
	digits := gzipped(t, strings.Repeat("1", 64<<20)+" 1\n")
	var err error
	grew := allocatedBy(func() { _, err = ReadMask(bytes.NewReader(digits), Options{}) })
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	if len(err.Error()) > 200 {
		t.Errorf("error echoes the input: %d bytes", len(err.Error()))
	}
	if grew > hostileAllocLimit {
		t.Errorf("allocated %d bytes", grew)
	}
}

func TestReadMaskRefusesMalformedHeaders(t *testing.T) {
	t.Parallel()
	for _, header := range []string{"5\n", "a b\n", "3 \n", "3 4 5\n", strings.Repeat("9", 70) + " 1\n"} {
		if _, err := ReadMask(bytes.NewReader(gzipped(t, header)), Options{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("header %q: err = %v, want ErrMalformed", header, err)
		}
	}
}

func TestDecodeMapNestingIsBounded(t *testing.T) {
	const levels = 2_000_000
	doc := `<map width="1" height="1">` + strings.Repeat("<a>", levels) + strings.Repeat("</a>", levels) + `</map>`
	var err error
	grew := allocatedBy(func() { _, err = DecodeMap(strings.NewReader(doc), TMX, Options{}) })
	if !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
	if grew > hostileAllocLimit {
		t.Errorf("allocated %d bytes for %d bytes of input", grew, len(doc))
	}
}

func TestDecodeMapAcceptsOrdinaryNesting(t *testing.T) {
	t.Parallel()
	doc := `<map width="1" height="1"><layer name="a" width="1" height="1"><data encoding="csv">1</data></layer></map>`
	m, err := DecodeMap(strings.NewReader(doc), TMX, Options{})
	if err != nil || len(m.Layers) != 1 || len(m.Layers[0].Tiles) != 1 {
		t.Fatalf("map = %+v, err = %v", m, err)
	}
}

// image/png allocates 8 bytes a pixel for 16-bit colour, so the pixel cap shrinks to keep the allocation within what
// MaxPixels 8-bit pixels cost.
func TestDecodePNGSixteenBitAllocationIsCapped(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA64(image.Rect(0, 0, 1000, 700))); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	var err error
	grew := allocatedBy(func() { _, err = DecodePNG(bytes.NewReader(raw), Options{MaxPixels: 1_000_000}) })
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if grew > hostileAllocLimit {
		t.Errorf("allocated %d bytes", grew)
	}
	if _, err := DecodePNG(bytes.NewReader(raw), Options{MaxPixels: 1_400_000}); err != nil {
		t.Errorf("700000 pixels under a 1.4M cap: %v", err)
	}
}

// A wide 8-bit RGBA image at the pixel cap must not allocate two full-width rows
// before falling back: those rows are ~8 bytes/pixel, past the 4×MaxPixels budget.
func TestDecodePNGWideRGBARowDoesNotAllocatePastStdBudget(t *testing.T) {
	const n = 4_000_000
	raw := []byte(pngSignature)
	raw = append(raw, pngChunk("IHDR", ihdr(n, 1, 8, 6, 0))...)
	raw = append(raw, pngChunk("IDAT", []byte{0x78, 0x9c})...)
	raw = append(raw, pngChunk("IEND", nil)...)
	grew := allocatedBy(func() { _, _ = DecodePNG(bytes.NewReader(raw), Options{MaxPixels: n}) })
	if grew > 64<<20 {
		t.Errorf("allocated %d bytes", grew)
	}
}

// A chunk length with the high bit set is negative as a 32-bit int.
func TestDecodePNGHugeChunkLengthDoesNotPanic(t *testing.T) {
	t.Parallel()
	raw := []byte(pngSignature)
	raw = append(raw, pngChunk("IHDR", ihdr(1, 1, 8, 6, 0))...)
	raw = append(raw, 0x80, 0, 0, 0, 't', 'E', 'X', 't', 0, 0, 0, 0, 0, 0, 0, 0)
	if _, err := DecodePNG(bytes.NewReader(raw), Options{}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}
