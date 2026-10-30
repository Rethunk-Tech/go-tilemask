package tilemask

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
)

func masksEqual(a, b *Mask) bool {
	return a.Width == b.Width && a.Height == b.Height && bytes.Equal(a.bits, b.bits)
}

// Both decoders must agree on every pixel for each colour type, with and without tRNS, under each filter.
func TestFastPathMatchesImagePNG(t *testing.T) {
	t.Parallel()
	const w, h = 13, 9
	for _, color := range []byte{0, 2, 3, 4, 6} {
		trnsCases := map[string][]byte{"none": nil}
		switch color {
		case 0:
			trnsCases["tRNS"] = []byte{0, 2}
		case 2:
			trnsCases["tRNS"] = []byte{0, 255, 0, 0, 0, 1}
		case 3:
			trnsCases["tRNS"] = []byte{0, 255, 0, 7}
		}
		for name, trns := range trnsCases {
			for filter := range byte(5) {
				t.Run(fmt.Sprintf("type%d/%s/filter%d", color, name, filter), func(t *testing.T) {
					t.Parallel()
					spec := pngSpec{w: w, h: h, color: color, trns: trns, filter: filter}
					spec.px = pseudoBytes(w*h*pngChannels(color), uint32(color)*31+uint32(filter))
					if color == 3 {
						spec.palette = pseudoBytes(3*8, 5)
						for i := range spec.px {
							spec.px[i] %= 10
						}
					}
					raw := buildPNG(t, spec)
					fast, err := fastPNG(raw, DefaultMaxPixels)
					if err != nil {
						t.Fatalf("fast path refused a plain 8-bit image: %v", err)
					}
					std, err := stdPNG(raw, DefaultMaxPixels)
					if err != nil {
						t.Fatal(err)
					}
					if !masksEqual(fast, std) {
						t.Fatalf("fast path and image/png disagree\nfast %x\nstd  %x", fast.bits, std.bits)
					}
				})
			}
		}
	}
}

func TestDecodePNGSetsNonzeroAlphaOnly(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 9})
	img.SetNRGBA(1, 0, color.NRGBA{A: 1})
	img.SetNRGBA(2, 0, color.NRGBA{A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	m, err := DecodePNG(&buf, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if m.At(0, 0) || !m.At(1, 0) || !m.At(2, 0) {
		t.Fatalf("mask = %v %v %v, want false true true", m.At(0, 0), m.At(1, 0), m.At(2, 0))
	}
	if m.At(-1, 0) || m.At(3, 0) || m.At(0, 1) || m.At(0, -1) {
		t.Fatal("a pixel outside the bounds was set")
	}
}

// image/png returns 16-bit images as NRGBA64, which the fast path leaves to it; the lowest nonzero alpha still counts.
func TestSixteenBitAlphaGoesThroughImagePNG(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA64(image.Rect(0, 0, 2, 1))
	img.SetNRGBA64(0, 0, color.NRGBA64{A: 0})
	img.SetNRGBA64(1, 0, color.NRGBA64{A: 1})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	m, err := DecodePNG(&buf, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if m.At(0, 0) || !m.At(1, 0) {
		t.Fatalf("mask = %v %v, want false true", m.At(0, 0), m.At(1, 0))
	}
}

type lowOpacityImage struct{}

func (lowOpacityImage) ColorModel() color.Model { return color.RGBA64Model }
func (lowOpacityImage) Bounds() image.Rectangle { return image.Rect(0, 0, 1, 1) }
func (lowOpacityImage) At(int, int) color.Color { return color.RGBA64{A: 1} }

func TestHasAlphaKeepsLowOpacity(t *testing.T) {
	t.Parallel()
	if !hasAlpha(lowOpacityImage{}, 0, 0) {
		t.Fatal("low nonzero alpha was treated as transparent")
	}
}

func TestDecodePNGRefusesWhatIsNotAPNG(t *testing.T) {
	t.Parallel()
	good := buildPNG(t, pngSpec{w: 2, h: 2, color: 6, px: pseudoBytes(16, 1)})
	for name, raw := range map[string][]byte{
		"empty":         nil,
		"text":          []byte("not a png at all"),
		"truncated":     good[:len(good)-20],
		"bad chunk len": append([]byte(pngSignature), 0xff, 0xff, 0xff, 0xff, 'I', 'H', 'D', 'R'),
		"no ihdr":       append([]byte(pngSignature), pngChunk("IEND", nil)...),
	} {
		if _, err := DecodePNG(bytes.NewReader(raw), Options{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}

// The fast path skips what a mask does not need; each skip errs toward more coverage, and this pins them.
func TestFastPathLeniency(t *testing.T) {
	t.Parallel()
	head := append([]byte(pngSignature), pngChunk("IHDR", ihdr(2, 2, 8, 2, 0))...)
	m, err := DecodePNG(bytes.NewReader(head), Options{})
	if err != nil || !m.At(0, 0) || !m.At(1, 1) {
		t.Fatalf("an opaque RGB image without IDAT: %v", err)
	}
	spec := pngSpec{w: 2, h: 1, color: 3, trns: []byte{0}, px: []byte{0, 1}}
	m, err = DecodePNG(bytes.NewReader(buildPNG(t, spec)), Options{})
	if err != nil || m.At(0, 0) || !m.At(1, 0) {
		t.Fatalf("a palette image without PLTE: %v", err)
	}
}

func TestHugeIHDRIsRefusedBeforeAllocation(t *testing.T) {
	t.Parallel()
	maxPlus := uint32(DefaultMaxPixels + 1)
	for _, d := range []struct{ w, h uint32 }{
		{30000, 30000}, {0x7fffffff, 0x7fffffff}, {0xffffffff, 0xffffffff}, {1, 0xffffffff}, {maxPlus, 1},
	} {
		for _, bitDepth := range []byte{8, 16} {
			raw := append([]byte(pngSignature), pngChunk("IHDR", ihdr(d.w, d.h, bitDepth, 2, 0))...)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			if _, err := DecodePNG(bytes.NewReader(raw), Options{}); !errors.Is(err, ErrTooLarge) {
				t.Errorf("%d x %d at %d bits: err = %v, want ErrTooLarge", d.w, d.h, bitDepth, err)
			}
			runtime.ReadMemStats(&after)
			if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<20 {
				t.Errorf("refusing %d x %d allocated %d MiB", d.w, d.h, grew>>20)
			}
		}
	}
}

func TestMaxPixelsOptionIsExact(t *testing.T) {
	t.Parallel()
	raw := buildPNG(t, pngSpec{w: 4, h: 3, color: 6, px: pseudoBytes(48, 2)})
	if _, err := DecodePNG(bytes.NewReader(raw), Options{MaxPixels: 12}); err != nil {
		t.Fatalf("12 pixels under a 12 pixel cap: %v", err)
	}
	if _, err := DecodePNG(bytes.NewReader(raw), Options{MaxPixels: 11}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("12 pixels under an 11 pixel cap: %v", err)
	}
}

func TestAreaWithinDoesNotWrap(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		w, h int
		want bool
	}{
		{10_000, 10_000, true},
		{10_001, 10_000, false},
		{0, 1 << 62, true},
		{0xffffffff, 0xffffffff, false},
		{1 << 62, 2, false},
		{-1, 1, false},
	} {
		if got := areaWithin(c.w, c.h, DefaultMaxPixels); got != c.want {
			t.Errorf("areaWithin(%d, %d) = %v, want %v", c.w, c.h, got, c.want)
		}
	}
}

func TestMaskSetAllLeavesPaddingClear(t *testing.T) {
	t.Parallel()
	m := newMask(3, 3)
	m.setAll()
	if len(m.bits) != 2 || m.bits[0] != 0xff || m.bits[1] != 1 {
		t.Fatalf("bits = %08b, want 11111111 00000001", m.bits)
	}
}
