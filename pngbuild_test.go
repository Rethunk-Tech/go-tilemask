package tilemask

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// pngSpec describes a hand-built 8-bit PNG: px holds the unfiltered rows, w*channels bytes each, and every row is
// written with the one filter type, so each unfilter branch is reached on its own.
type pngSpec struct {
	w, h    int
	color   byte
	palette []byte
	trns    []byte
	filter  byte
	px      []byte
}

func pngChunk(kind string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, kind...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[4:]))
}

func ihdr(w, h uint32, bitDepth, color, interlace byte) []byte {
	d := binary.BigEndian.AppendUint32(nil, w)
	d = binary.BigEndian.AppendUint32(d, h)
	return append(d, bitDepth, color, 0, 0, interlace)
}

func filterRow(filter byte, cur, prior []byte, cpp int) []byte {
	out := make([]byte, 1+len(cur))
	out[0] = filter
	for i := range cur {
		var a, b, c int
		if i >= cpp {
			a, c = int(cur[i-cpp]), int(prior[i-cpp])
		}
		b = int(prior[i])
		var pred int
		switch filter {
		case 1:
			pred = a
		case 2:
			pred = b
		case 3:
			pred = (a + b) / 2
		case 4:
			p := a + b - c
			pa, pb, pc := abs(p-a), abs(p-b), abs(p-c)
			switch {
			case pa <= pb && pa <= pc:
				pred = a
			case pb <= pc:
				pred = b
			default:
				pred = c
			}
		}
		out[1+i] = cur[i] - byte(pred)
	}
	return out
}

func buildPNG(t testing.TB, s pngSpec) []byte {
	t.Helper()
	cpp := pngChannels(s.color)
	stride := s.w * cpp
	var rows []byte
	prior := make([]byte, stride)
	for y := range s.h {
		cur := s.px[y*stride : (y+1)*stride]
		rows = append(rows, filterRow(s.filter, cur, prior, cpp)...)
		prior = cur
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out := []byte(pngSignature)
	out = append(out, pngChunk("IHDR", ihdr(uint32(s.w), uint32(s.h), 8, s.color, 0))...)
	if s.palette != nil {
		out = append(out, pngChunk("PLTE", s.palette)...)
	}
	if s.trns != nil {
		out = append(out, pngChunk("tRNS", s.trns)...)
	}
	out = append(out, pngChunk("IDAT", z.Bytes())...)
	return append(out, pngChunk("IEND", nil)...)
}

// pseudoBytes returns n deterministic bytes drawn mostly from a few values, so transparent and tRNS-matching
// samples occur often.
func pseudoBytes(n int, seed uint32) []byte {
	common := []byte{0, 1, 2, 255, 128}
	out := make([]byte, n)
	for i := range out {
		seed = seed*1664525 + 1013904223
		if seed>>28 < 9 {
			out[i] = common[seed>>8%uint32(len(common))]
		} else {
			out[i] = byte(seed >> 16)
		}
	}
	return out
}
