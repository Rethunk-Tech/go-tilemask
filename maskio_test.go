package tilemask

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"runtime"
	"testing"
)

func gzipped(t testing.TB, payload string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The golden file holds the on-disk layout: ReadMask must read it and WriteTo must reproduce it byte for byte.
func TestMaskFileGolden(t *testing.T) {
	t.Parallel()
	golden, err := os.ReadFile("testdata/mask-gzip-19x5.gz")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ReadMask(bytes.NewReader(golden), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if want := patternMask(19, 5); !masksEqual(m, want) {
		t.Fatalf("golden mask differs from the pattern it was written from\ngot  %x\nwant %x", m.bits, want.bits)
	}
	var out bytes.Buffer
	n, err := m.WriteTo(&out)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(out.Len()) || !bytes.Equal(out.Bytes(), golden) {
		t.Fatalf("WriteTo = %d bytes %x, want %x", n, out.Bytes(), golden)
	}
}

// A mask wider than 65536 px is within the pixel cap, so what WriteTo wrote reads back.
func TestMaskFileRoundTripsAWideMask(t *testing.T) {
	t.Parallel()
	m := newMask(70000, 2)
	m.bits[0], m.bits[len(m.bits)-1] = 0b1010_0101, 0b1100_0011
	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMask(&buf, Options{})
	if err != nil || !masksEqual(got, m) {
		t.Fatalf("round trip: %v", err)
	}
}

// A header could claim any size; it is held to the cap before its claim is allocated.
func TestReadMaskRefusesAnOversizedHeaderBeforeAllocating(t *testing.T) {
	t.Parallel()
	for _, header := range []string{"65536 65536\n", "4294967296 4294967296\n", "9223372036854775807 2\n"} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if _, err := ReadMask(bytes.NewReader(gzipped(t, header)), Options{}); !errors.Is(err, ErrTooLarge) {
			t.Errorf("header %q: err = %v, want ErrTooLarge", header, err)
		}
		runtime.ReadMemStats(&after)
		if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<20 {
			t.Errorf("refusing header %q allocated %d MiB", header, grew>>20)
		}
	}
}

func TestReadMaskUsesTheCallersCap(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if _, err := patternMask(4, 3).WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMask(bytes.NewReader(buf.Bytes()), Options{MaxPixels: 12}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMask(bytes.NewReader(buf.Bytes()), Options{MaxPixels: 11}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestReadMaskRefusesDamage(t *testing.T) {
	t.Parallel()
	var good bytes.Buffer
	if _, err := patternMask(19, 5).WriteTo(&good); err != nil {
		t.Fatal(err)
	}
	flipped := bytes.Clone(good.Bytes())
	flipped[len(flipped)-8] ^= 0xff // the CRC-32 in the gzip trailer
	for name, raw := range map[string][]byte{
		"flipped trailer": flipped,
		"truncated":       good.Bytes()[:good.Len()-12],
		"not gzip":        []byte("plain text"),
		"short bits":      gzipped(t, "19 5\nab"),
		"no header":       gzipped(t, "hello"),
		"zero width":      gzipped(t, "0 5\n"),
		"negative":        gzipped(t, "-3 5\n"),
	} {
		if _, err := ReadMask(bytes.NewReader(raw), Options{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}

func TestReadMaskClearsPaddingBits(t *testing.T) {
	t.Parallel()
	m, err := ReadMask(bytes.NewReader(gzipped(t, "3 3\n\xff\xff")), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := newMask(3, 3)
	want.setAll()
	if !masksEqual(m, want) {
		t.Fatalf("bits = %08b, want 11111111 00000001", m.bits)
	}
}

func TestWriteToRefusesWhatReadMaskWould(t *testing.T) {
	t.Parallel()
	for _, m := range []*Mask{{}, {Width: 3, Height: 3}} {
		if _, err := m.WriteTo(&bytes.Buffer{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%dx%d: err = %v, want ErrMalformed", m.Width, m.Height, err)
		}
	}
}
