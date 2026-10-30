package tilemask

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

type tmxMapDocument struct {
	XMLName xml.Name   `xml:"map"`
	Width   int        `xml:"width,attr"`
	Height  int        `xml:"height,attr"`
	Layers  []tmxLayer `xml:"layer"`
}

type tmxLayer struct {
	Name   string  `xml:"name,attr"`
	Width  int     `xml:"width,attr"`
	Height int     `xml:"height,attr"`
	Data   tmxData `xml:"data"`
}

type tmxData struct {
	Encoding    string     `xml:"encoding,attr"`
	Compression string     `xml:"compression,attr"`
	Text        string     `xml:",chardata"`
	Tiles       []tmxTile  `xml:"tile"`
	Chunks      []tmxChunk `xml:"chunk"`
}

type tmxTile struct {
	GID uint32 `xml:"gid,attr"`
}

type tmxChunk struct {
	X      int       `xml:"x,attr"`
	Y      int       `xml:"y,attr"`
	Width  int       `xml:"width,attr"`
	Height int       `xml:"height,attr"`
	Text   string    `xml:",chardata"`
	Tiles  []tmxTile `xml:"tile"`
}

func parseTMX(r io.Reader) (width, height int, layers []layerSpec, err error) {
	var doc tmxMapDocument
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return 0, 0, nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	for _, l := range doc.Layers {
		spec := layerSpec{name: l.Name}
		if len(l.Data.Chunks) == 0 {
			spec.blocks = []block{{w: l.Width, h: l.Height, values: tmxValues(l.Data.Text, l.Data.Tiles, l.Data)}}
		}
		for _, c := range l.Data.Chunks {
			spec.blocks = append(spec.blocks, block{x: c.X, y: c.Y, w: c.Width, h: c.Height, values: tmxValues(c.Text, c.Tiles, l.Data)})
		}
		layers = append(layers, spec)
	}
	return doc.Width, doc.Height, layers, nil
}

// tmxValues prefers a <tile> list to text, as a layer holds one or the other.
func tmxValues(text string, tiles []tmxTile, d tmxData) func(int) ([]int, error) {
	return func(limit int) ([]int, error) {
		if len(tiles) == 0 {
			return decodeValues(text, d.Encoding, d.Compression, limit)
		}
		if len(tiles) > limit {
			return nil, errBeyondDeclared
		}
		out := make([]int, len(tiles))
		for i, t := range tiles {
			out[i] = int(t.GID)
		}
		return out, nil
	}
}

type tmjMapDocument struct {
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Layers []tmjLayer `json:"layers"`
}

type tmjLayer struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	Compression string          `json:"compression"`
	Data        json.RawMessage `json:"data"`
	Chunks      []tmjChunk      `json:"chunks"`
}

type tmjChunk struct {
	X      int             `json:"x"`
	Y      int             `json:"y"`
	Width  int             `json:"width"`
	Height int             `json:"height"`
	Data   json.RawMessage `json:"data"`
}

func parseTMJ(r io.Reader) (width, height int, layers []layerSpec, err error) {
	var doc tmjMapDocument
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return 0, 0, nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	for _, l := range doc.Layers {
		if l.Type != "" && !strings.EqualFold(l.Type, "tilelayer") {
			continue
		}
		spec := layerSpec{name: l.Name}
		if len(l.Chunks) == 0 {
			// A layer without a width takes the map's.
			w := l.Width
			if w <= 0 {
				w = doc.Width
			}
			spec.blocks = []block{{w: w, h: l.Height, values: tmjValues(l.Data, l.Compression)}}
		}
		for _, c := range l.Chunks {
			spec.blocks = append(spec.blocks, block{x: c.X, y: c.Y, w: c.Width, h: c.Height, values: tmjValues(c.Data, l.Compression)})
		}
		layers = append(layers, spec)
	}
	return doc.Width, doc.Height, layers, nil
}

// tmjValues reads a JSON array of GIDs, or a base64 string through the same decoder as TMX.
func tmjValues(raw json.RawMessage, compression string) func(int) ([]int, error) {
	return func(limit int) ([]int, error) {
		raw := bytes.TrimSpace(raw)
		switch {
		case len(raw) == 0 || string(raw) == "null":
			return nil, nil
		case raw[0] == '"':
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
			}
			return decodeValues(text, "base64", compression, limit)
		case raw[0] == '[':
			return tmjInts(raw, limit)
		default:
			return nil, fmt.Errorf("%w: layer data is neither an array nor a string", ErrMalformed)
		}
	}
}

// tmjInts reads values one at a time, so an array longer than the declared size is refused before it is held.
func tmjInts(raw []byte, limit int) ([]int, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	var out []int
	for dec.More() {
		if len(out) == limit {
			return nil, errBeyondDeclared
		}
		var v int
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		out = append(out, v)
	}
	return out, nil
}

var errBeyondDeclared = fmt.Errorf("%w: more values than the declared size", ErrMalformed)

// decodeValues reads CSV text, or base64 text with no compression, zlib or gzip, into at most limit GIDs.
func decodeValues(text, encoding, compression string, limit int) ([]int, error) {
	text = strings.TrimSpace(text)
	if strings.EqualFold(encoding, "csv") {
		var out []int
		for value := range strings.SplitSeq(text, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%w: tile %q", ErrMalformed, value)
			}
			if len(out) == limit {
				return nil, errBeyondDeclared
			}
			out = append(out, n)
		}
		return out, nil
	}
	if text == "" {
		return nil, nil
	}
	if !strings.EqualFold(encoding, "base64") {
		return nil, ErrUnsupported
	}
	encoded, err := base64.StdEncoding.DecodeString(strings.Map(dropSpace, text))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	data, err := inflate(encoded, compression, int64(limit)*4)
	if err != nil {
		return nil, err
	}
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("%w: data is not whole 32-bit tiles", ErrMalformed)
	}
	out := make([]int, len(data)/4)
	for i := range out {
		out[i] = int(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return out, nil
}

func dropSpace(r rune) rune {
	if unicode.IsSpace(r) {
		return -1
	}
	return r
}

// inflate returns data decompressed, and errBeyondDeclared when it is longer than limit bytes.
func inflate(data []byte, compression string, limit int64) ([]byte, error) {
	var reader io.Reader
	switch strings.ToLower(strings.TrimSpace(compression)) {
	case "":
		reader = bytes.NewReader(data)
	case "zlib":
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		defer func() { _ = zr.Close() }()
		reader = zr
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		defer func() { _ = zr.Close() }()
		reader = zr
	default:
		return nil, ErrUnsupported
	}
	out, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	if int64(len(out)) > limit {
		return nil, errBeyondDeclared
	}
	return out, nil
}
