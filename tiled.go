package tilemask

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
)

// Format is a Tiled map file format.
type Format uint8

// The formats DecodeMap and MapSize read.
const (
	TMX Format = iota + 1 // XML
	TMJ                   // JSON
)

// Map is the occupied tiles of a Tiled map.
type Map struct {
	// Width and Height are the header's size in tiles, grown to cover every occupied tile.
	Width, Height int
	// Layers are the top-level tile layers in document order. Group layers are not entered.
	Layers []Layer
}

// Layer is one tile layer.
type Layer struct {
	// Name is verbatim: no case folding or trimming.
	Name string
	// Tiles are the occupied tiles (GID not zero, flip bits included), in the map's coordinates: an infinite map's
	// chunk offsets are applied.
	Tiles []image.Point
}

// block is a rectangle of tile values, a whole layer or one chunk of an infinite map.
type block struct {
	x, y, w, h int
	// values returns at most limit values, and ErrMalformed when the data holds more.
	values func(limit int) ([]int, error)
}

type layerSpec struct {
	name   string
	blocks []block
}

// DecodeMap reads r until the document ends and returns its occupied tiles.
//
// The declared sizes of every layer and chunk together must be within opts.MaxTiles, checked before any data
// inflates (ErrTooLarge). A layer or chunk whose data holds more values than its declared width times height, or
// whose compressed data inflates past it, is ErrMalformed, so memory use does not depend on the size of r. An
// encoding or compression other than CSV, tile lists and base64 with none, zlib or gzip is ErrUnsupported, and fails
// the whole map.
func DecodeMap(r io.Reader, f Format, opts Options) (Map, error) {
	var (
		width, height int
		layers        []layerSpec
		err           error
	)
	switch f {
	case TMX:
		width, height, layers, err = parseTMX(r)
	case TMJ:
		width, height, layers, err = parseTMJ(r)
	default:
		return Map{}, ErrUnsupported
	}
	if err != nil {
		return Map{}, err
	}
	if err := checkDeclared(layers, opts.maxTiles()); err != nil {
		return Map{}, err
	}
	out := Map{Width: max(width, 0), Height: max(height, 0)}
	for _, spec := range layers {
		layer := Layer{Name: spec.name}
		for _, b := range spec.blocks {
			values, err := b.values(b.w * b.h)
			if err != nil {
				return Map{}, err
			}
			for i, v := range values {
				if v != 0 {
					layer.Tiles = append(layer.Tiles, image.Pt(b.x+i%b.w, b.y+i/b.w))
				}
			}
		}
		out.Layers = append(out.Layers, layer)
	}
	for _, layer := range out.Layers {
		for _, p := range layer.Tiles {
			out.Width = max(out.Width, p.X+1)
			out.Height = max(out.Height, p.Y+1)
		}
	}
	return out, nil
}

// checkDeclared sums every block's declared size before anything inflates. Chunk offsets are held to int32, the
// range Tiled writes, so adding a tile's offset within its block cannot wrap.
func checkDeclared(layers []layerSpec, maxTiles int64) error {
	var total int64
	for _, spec := range layers {
		for _, b := range spec.blocks {
			if b.w < 0 || b.h < 0 || b.x < math.MinInt32 || b.x > math.MaxInt32 || b.y < math.MinInt32 || b.y > math.MaxInt32 {
				return fmt.Errorf("%w: layer %q block %d,%d size %d x %d", ErrMalformed, spec.name, b.x, b.y, b.w, b.h)
			}
			if !areaWithin(b.w, b.h, maxTiles-total) {
				return ErrTooLarge
			}
			total += int64(b.w) * int64(b.h)
		}
	}
	return nil
}

// MapSize returns the map's width and height in tiles from its header alone. TMX stops reading at the root
// element; TMJ reads the top-level object. A missing or non-positive size is ErrMalformed.
func MapSize(r io.Reader, f Format) (w, h int, err error) {
	switch f {
	case TMX:
		w, h, err = tmxSize(r)
	case TMJ:
		var m struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if jerr := json.NewDecoder(r).Decode(&m); jerr != nil {
			return 0, 0, fmt.Errorf("%w: %w", ErrMalformed, jerr)
		}
		w, h = m.Width, m.Height
	default:
		return 0, 0, ErrUnsupported
	}
	if err == nil && (w <= 0 || h <= 0) {
		err = fmt.Errorf("%w: map size %d x %d", ErrMalformed, w, h)
	}
	if err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

func tmxSize(r io.Reader) (w, h int, err error) {
	dec := xml.NewDecoder(r)
	for {
		tok, terr := dec.Token()
		if terr != nil {
			return 0, 0, fmt.Errorf("%w: %w", ErrMalformed, terr)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "map" {
			return 0, 0, fmt.Errorf("%w: root element %q", ErrMalformed, start.Name.Local)
		}
		for _, a := range start.Attr {
			// A value that is not a number leaves the size at zero, which the caller refuses.
			switch a.Name.Local {
			case "width":
				w, _ = strconv.Atoi(a.Value)
			case "height":
				h, _ = strconv.Atoi(a.Value)
			}
		}
		return w, h, nil
	}
}
