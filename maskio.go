package tilemask

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// maxHeaderBytes bounds the "W H\n" line: two 64-bit decimals, a space and a newline fit well inside it.
const maxHeaderBytes = 64

// readMaskHeader reads the header line one byte at a time up to maxHeaderBytes, so a stream with no newline cannot
// make the reader buffer it, and never echoes the input into an error.
func readMaskHeader(br *bufio.Reader) (w, h int, err error) {
	line := make([]byte, 0, maxHeaderBytes)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return 0, 0, fmt.Errorf("%w: header: %w", ErrMalformed, err)
		}
		if b == '\n' {
			break
		}
		if len(line) == maxHeaderBytes {
			return 0, 0, fmt.Errorf("%w: header longer than %d bytes", ErrMalformed, maxHeaderBytes)
		}
		line = append(line, b)
	}
	ws, hs, ok := strings.Cut(string(line), " ")
	if !ok {
		return 0, 0, fmt.Errorf("%w: header has no size pair", ErrMalformed)
	}
	w, errW := strconv.Atoi(ws)
	h, errH := strconv.Atoi(hs)
	if errW != nil || errH != nil {
		return 0, 0, fmt.Errorf("%w: header sizes are not integers", ErrMalformed)
	}
	return w, h, nil
}

// WriteTo writes the mask as gzip (best speed) of "W H\n" followed by the packed bits, and returns the compressed
// bytes written. A mask with a non-positive size is refused, since ReadMask would refuse it.
func (m *Mask) WriteTo(w io.Writer) (int64, error) {
	if m.Width <= 0 || m.Height <= 0 || len(m.bits) != (m.Width*m.Height+7)/8 {
		return 0, fmt.Errorf("%w: mask %d x %d", ErrMalformed, m.Width, m.Height)
	}
	cw := &countWriter{w: w}
	zw, err := gzip.NewWriterLevel(cw, gzip.BestSpeed)
	if err != nil {
		return 0, fmt.Errorf("tilemask: write mask: %w", err)
	}
	if _, err := fmt.Fprintf(zw, "%d %d\n", m.Width, m.Height); err != nil {
		return cw.n, fmt.Errorf("tilemask: write mask: %w", err)
	}
	if _, err := zw.Write(m.bits); err != nil {
		return cw.n, fmt.Errorf("tilemask: write mask: %w", err)
	}
	if err := zw.Close(); err != nil {
		return cw.n, fmt.Errorf("tilemask: write mask: %w", err)
	}
	return cw.n, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// ReadMask reads a mask WriteTo wrote. The size in the header is held to opts.MaxPixels before the bits are
// allocated, so a crafted header cannot claim more than a decode would have allowed, and every mask written under
// that cap reads back. The gzip trailer is verified, so a torn write is refused as ErrMalformed.
func ReadMask(r io.Reader, opts Options) (*Mask, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	br := bufio.NewReader(zr)
	w, h, err := readMaskHeader(br)
	if err != nil {
		return nil, err
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%w: mask %d x %d", ErrMalformed, w, h)
	}
	if !areaWithin(w, h, opts.maxPixels()) {
		return nil, ErrTooLarge
	}
	m := newMask(w, h)
	if _, err := io.ReadFull(br, m.bits); err != nil {
		return nil, fmt.Errorf("%w: bits: %w", ErrMalformed, err)
	}
	if rem := w * h & 7; rem != 0 {
		m.bits[len(m.bits)-1] &= 1<<rem - 1
	}
	if _, err := io.Copy(io.Discard, br); err != nil {
		return nil, fmt.Errorf("%w: trailer: %w", ErrMalformed, err)
	}
	return m, nil
}
