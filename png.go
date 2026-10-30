package tilemask

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
)

const pngSignature = "\x89PNG\r\n\x1a\n"

// errFallback sends an input the fast path cannot read to image/png.
var errFallback = errors.New("tilemask: use image/png")

// DecodePNG reads r to the end and returns the mask of pixels whose alpha is not zero.
//
// The size the IHDR claims is checked against opts.MaxPixels before any pixel buffer exists. Eight-bit,
// non-interlaced images are read by a fast path that skips what a mask does not need: it checks no chunk CRC and no
// Adler-32 trailer, needs no PLTE for colour type 3, needs no IDAT for an 8-bit grey or RGB image without tRNS
// (every pixel is set), and ignores data after the rows it needs. Each of those errs toward more coverage, never
// less. Everything else, and an image the fast path cannot inflate, goes through image/png.
func DecodePNG(r io.Reader, opts Options) (*Mask, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("tilemask: read png: %w", err)
	}
	m, err := fastPNG(raw, opts.maxPixels())
	if errors.Is(err, errFallback) {
		return stdPNG(raw, opts.maxPixels())
	}
	return m, err
}

type pngHeader struct {
	w, h                           int
	bitDepth, colorType, interlace byte
	trns, idat                     []byte
	seen                           bool
}

func parsePNGChunks(raw []byte) (pngHeader, error) {
	var hd pngHeader
	for i := 8; i+12 <= len(raw); {
		n := int(binary.BigEndian.Uint32(raw[i:]))
		i += 4
		if i+4+n+4 > len(raw) {
			return hd, ErrMalformed
		}
		kind := string(raw[i : i+4])
		data := raw[i+4 : i+4+n]
		i += 4 + n + 4
		switch kind {
		case "IHDR":
			if n < 13 {
				return hd, ErrMalformed
			}
			hd.w = int(binary.BigEndian.Uint32(data[0:4]))
			hd.h = int(binary.BigEndian.Uint32(data[4:8]))
			hd.bitDepth, hd.colorType, hd.interlace = data[8], data[9], data[12]
			hd.seen = true
		case "tRNS":
			hd.trns = data
		case "IDAT":
			hd.idat = append(hd.idat, data...)
		case "IEND":
			i = len(raw)
		}
	}
	return hd, nil
}

// fastPNG returns errFallback for what image/png should decide.
func fastPNG(raw []byte, maxPixels int64) (*Mask, error) {
	if len(raw) < 8 || string(raw[:8]) != pngSignature {
		return nil, fmt.Errorf("%w: not a png", ErrMalformed)
	}
	hd, err := parsePNGChunks(raw)
	if err != nil {
		return nil, err
	}
	if hd.seen && !areaWithin(hd.w, hd.h, maxPixels) {
		return nil, ErrTooLarge
	}
	if !hd.seen || hd.w <= 0 || hd.h <= 0 || hd.interlace != 0 || hd.bitDepth != 8 {
		return nil, errFallback
	}
	m := newMask(hd.w, hd.h)
	if (hd.colorType == 0 || hd.colorType == 2) && len(hd.trns) == 0 {
		m.setAll()
		return m, nil
	}
	if !inflatePNG(m, hd) {
		return nil, errFallback
	}
	return m, nil
}

func pngChannels(colorType byte) int {
	switch colorType {
	case 0, 3:
		return 1
	case 2:
		return 3
	case 4:
		return 2
	case 6:
		return 4
	default:
		return 0
	}
}

// inflatePNG fills m from the unfiltered rows, and reports false when the stream cannot supply them all.
func inflatePNG(m *Mask, hd pngHeader) bool {
	cpp := pngChannels(hd.colorType)
	if cpp == 0 {
		return false
	}
	zr, err := zlib.NewReader(bytes.NewReader(hd.idat))
	if err != nil {
		return false
	}
	defer func() { _ = zr.Close() }()
	stride := hd.w*cpp + 1
	row := make([]byte, stride)
	prev := make([]byte, stride)
	for y := range hd.h {
		if _, err := io.ReadFull(zr, row); err != nil || !pngUnfilter(row, prev, cpp) {
			return false
		}
		base := y * hd.w
		pix := row[1:]
		for x := range hd.w {
			if pngAlphaSet(hd, pix, x) {
				m.set(base + x)
			}
		}
		copy(prev, row)
	}
	return true
}

// pngAlphaSet reports whether pixel x of an unfiltered row has alpha not zero. tRNS for colour types 0 and 2
// compares the low byte only, as image/png does for 8-bit images.
func pngAlphaSet(hd pngHeader, pix []byte, x int) bool {
	trns := hd.trns
	switch hd.colorType {
	case 0:
		return len(trns) < 2 || pix[x] != trns[1]
	case 2:
		off := x * 3
		return len(trns) < 6 || pix[off] != trns[1] || pix[off+1] != trns[3] || pix[off+2] != trns[5]
	case 3:
		if idx := int(pix[x]); idx < len(trns) {
			return trns[idx] != 0
		}
		return true
	case 4:
		return pix[x*2+1] != 0
	default:
		return pix[x*4+3] != 0
	}
}

func pngUnfilter(row, prev []byte, cpp int) bool {
	cur := row[1:]
	prior := prev[1:]
	switch row[0] {
	case 0:
	case 1:
		for i := cpp; i < len(cur); i++ {
			cur[i] += cur[i-cpp]
		}
	case 2:
		for i := range cur {
			cur[i] += prior[i]
		}
	case 3:
		for i := range cur {
			var a byte
			if i >= cpp {
				a = cur[i-cpp]
			}
			cur[i] += a>>1 + prior[i]>>1 + a&prior[i]&1
		}
	case 4:
		for i := range cur {
			var a, c byte
			if i >= cpp {
				a = cur[i-cpp]
				c = prior[i-cpp]
			}
			cur[i] += paeth(a, prior[i], c)
		}
	default:
		return false
	}
	return true
}

func paeth(a, b, c byte) byte {
	ia, ib, ic := int(a), int(b), int(c)
	p := ia + ib - ic
	pa, pb, pc := abs(p-ia), abs(p-ib), abs(p-ic)
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// stdPNG decodes through image/png, after the same size cap read from the header alone.
func stdPNG(raw []byte, maxPixels int64) (*Mask, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if !areaWithin(cfg.Width, cfg.Height, maxPixels) {
		return nil, ErrTooLarge
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	b := img.Bounds()
	m := newMask(b.Dx(), b.Dy())
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if hasAlpha(img, x, y) {
				m.set(i)
			}
			i++
		}
	}
	return m, nil
}

// hasAlpha is true for any nonzero alpha, however low and at any bit depth.
func hasAlpha(img image.Image, x, y int) bool {
	switch img := img.(type) {
	case *image.NRGBA:
		return img.Pix[img.PixOffset(x, y)+3] != 0
	case *image.RGBA:
		return img.Pix[img.PixOffset(x, y)+3] != 0
	default:
		_, _, _, a := img.At(x, y).RGBA()
		return a > 0
	}
}
