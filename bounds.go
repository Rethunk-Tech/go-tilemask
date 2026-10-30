package tilemask

import "errors"

const (
	// DefaultMaxPixels is the image size cap when Options.MaxPixels is zero.
	DefaultMaxPixels int64 = 100_000_000
	// DefaultMaxTiles is the map tile cap when Options.MaxTiles is zero.
	DefaultMaxTiles int64 = 4_000_000
)

var (
	// ErrTooLarge means a declared or decoded size exceeds its cap.
	ErrTooLarge = errors.New("tilemask: size exceeds the cap")
	// ErrUnsupported means a well-formed input uses an encoding or compression this package does not read.
	ErrUnsupported = errors.New("tilemask: unsupported encoding or compression")
	// ErrMalformed means the input is not the format it claims, or contradicts its own header.
	ErrMalformed = errors.New("tilemask: malformed input")
)

// Options holds the caps. A zero or negative field means its default.
type Options struct {
	// MaxPixels caps an image's width times height, checked before any pixel buffer is allocated.
	MaxPixels int64
	// MaxTiles caps a map's declared tiles, summed over every layer and chunk before any data inflates. No
	// layer or chunk may then hold more values than it declared, so it also caps the tiles decoded.
	MaxTiles int64
}

func (o Options) maxPixels() int64 {
	if o.MaxPixels <= 0 {
		return DefaultMaxPixels
	}
	return o.MaxPixels
}

func (o Options) maxTiles() int64 {
	if o.MaxTiles <= 0 {
		return DefaultMaxTiles
	}
	return o.MaxTiles
}

// areaWithin reports whether a w by h area is at most limit. It divides rather than multiplies, since dimensions an
// input declares can be large enough that the product wraps negative and passes.
func areaWithin(w, h int, limit int64) bool {
	if w < 0 || h < 0 || limit < 0 {
		return false
	}
	return h == 0 || int64(w) <= limit/int64(h)
}
