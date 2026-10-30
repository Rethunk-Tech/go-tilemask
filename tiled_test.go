package tilemask

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
)

func compressed(t testing.TB, kind string, data []byte) string {
	t.Helper()
	var buf bytes.Buffer
	switch kind {
	case "zlib":
		w := zlib.NewWriter(&buf)
		_, _ = w.Write(data)
		_ = w.Close()
	case "gzip":
		w := gzip.NewWriter(&buf)
		_, _ = w.Write(data)
		_ = w.Close()
	default:
		buf.Write(data)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func gids(values ...uint32) []byte {
	var out []byte
	for _, v := range values {
		out = binary.LittleEndian.AppendUint32(out, v)
	}
	return out
}

func decodeString(doc string, f Format, opts Options) (Map, error) {
	return DecodeMap(strings.NewReader(doc), f, opts)
}

func tmx(layers ...string) string {
	return `<map width="4" height="4">` + strings.Join(layers, "") + `</map>`
}

func tmxLayerOf(name string, w, h int, data string) string {
	return fmt.Sprintf(`<layer name=%q width="%d" height="%d">%s</layer>`, name, w, h, data)
}

func points(m Map, layer int) []image.Point { return m.Layers[layer].Tiles }

func TestDecodeMapReadsEveryEncoding(t *testing.T) {
	t.Parallel()
	want := []image.Point{{1, 0}, {0, 1}, {1, 1}}
	flipped := uint32(1<<31 | 5)
	values := []uint32{0, 7, 3, flipped}
	for _, c := range []struct {
		name string
		doc  string
		f    Format
	}{
		{"tmx csv", tmx(tmxLayerOf("L", 2, 2, `<data encoding="csv">0,7,
3,2147483653</data>`)), TMX},
		{"tmx tiles", tmx(tmxLayerOf("L", 2, 2, `<data><tile gid="0"/><tile gid="7"/><tile gid="3"/><tile gid="2147483653"/></data>`)), TMX},
		{"tmx plain base64", tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64">`+compressed(t, "", gids(values...))+`</data>`)), TMX},
		{"tmx zlib", tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64" compression="zlib">`+compressed(t, "zlib", gids(values...))+`</data>`)), TMX},
		{"tmx gzip", tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64" compression="gzip">`+compressed(t, "gzip", gids(values...))+`</data>`)), TMX},
		{"tmj array", `{"width":4,"height":4,"layers":[{"name":"L","type":"tilelayer","width":2,"height":2,"data":[0,7,3,2147483653]}]}`, TMJ},
		{"tmj base64", `{"width":4,"height":4,"layers":[{"name":"L","type":"tilelayer","width":2,"height":2,"encoding":"base64","data":"` + compressed(t, "", gids(values...)) + `"}]}`, TMJ},
		{"tmj zlib", `{"width":4,"height":4,"layers":[{"name":"L","width":2,"height":2,"encoding":"base64","compression":"zlib","data":"` + compressed(t, "zlib", gids(values...)) + `"}]}`, TMJ},
	} {
		m, err := decodeString(c.doc, c.f, Options{})
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(m.Layers) != 1 || m.Layers[0].Name != "L" || !slices.Equal(points(m, 0), want) {
			t.Errorf("%s: layers = %+v, want tiles %v", c.name, m.Layers, want)
		}
	}
}

func TestDecodeMapLayersAndSize(t *testing.T) {
	t.Parallel()
	doc := `<map width="2" height="2">
 <layer name=" Back " width="2" height="2"><data encoding="csv">0,0,0,0</data></layer>
 <group><layer name="Hidden" width="1" height="1"><data encoding="csv">1</data></layer></group>
 <layer name="Front" width="2" height="2"><data encoding="csv">1,0,0,0</data></layer>
</map>`
	m, err := decodeString(doc, TMX, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Layers) != 2 || m.Layers[0].Name != " Back " || m.Layers[1].Name != "Front" || len(m.Layers[0].Tiles) != 0 {
		t.Fatalf("layers = %+v", m.Layers)
	}
	// An infinite map's tiles sit at chunk offsets, and the size grows to cover them.
	inf := `<map width="1" height="1"><layer name="L" width="0" height="0"><data encoding="csv">
<chunk x="-16" y="32" width="2" height="2">0,1,0,0</chunk><chunk x="16" y="0" width="2" height="2">0,0,0,1</chunk></data></layer></map>`
	m, err = decodeString(inf, TMX, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []image.Point{{-15, 32}, {17, 1}}; !slices.Equal(points(m, 0), want) || m.Width != 18 || m.Height != 33 {
		t.Fatalf("tiles = %v size %d x %d", points(m, 0), m.Width, m.Height)
	}
	tmj := `{"width":3,"height":1,"layers":[
 {"type":"objectgroup","name":"objs","objects":[]},
 {"type":"tilelayer","name":"NoWidth","height":1,"data":[0,1,1]},
 {"type":"tilelayer","name":"Inf","chunks":[{"x":2,"y":4,"width":2,"height":1,"data":[1,0]}]}]}`
	m, err = decodeString(tmj, TMJ, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Layers) != 2 || !slices.Equal(points(m, 0), []image.Point{{1, 0}, {2, 0}}) ||
		!slices.Equal(points(m, 1), []image.Point{{2, 4}}) || m.Width != 3 || m.Height != 5 {
		t.Fatalf("layers = %+v size %d x %d", m.Layers, m.Width, m.Height)
	}
}

func TestDecodeMapRefusesInflationPastTheDeclaredSize(t *testing.T) {
	t.Parallel()
	bomb := compressed(t, "zlib", make([]byte, 1<<20))
	doc := func(w, h int) string {
		return tmx(tmxLayerOf("L", w, h, `<data encoding="base64" compression="zlib">`+bomb+`</data>`))
	}
	if _, err := decodeString(doc(2, 2), TMX, Options{}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("a zlib bomb in a 2x2 layer: %v", err)
	}
	if _, err := decodeString(doc(1<<20, 1<<20), TMX, Options{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("an absurd declared size: %v", err)
	}
	honest := tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64" compression="zlib">`+compressed(t, "zlib", gids(1, 0, 0, 1))+`</data>`))
	if m, err := decodeString(honest, TMX, Options{}); err != nil || len(points(m, 0)) != 2 {
		t.Fatalf("an honest layer: %+v %v", m, err)
	}
}

// An infinite map may hold any number of chunks, each within the per-chunk bound, so the cap covers the whole map
// before any chunk inflates.
func TestMapTileCapCoversTheWholeMap(t *testing.T) {
	t.Parallel()
	chunk := `<chunk x="0" y="0" width="1000" height="1000"></chunk>`
	layer := func(chunks ...string) string {
		return `<layer name="L" width="0" height="0"><data encoding="csv">` + strings.Join(chunks, "") + `</data></layer>`
	}
	if _, err := decodeString(tmx(layer(chunk, chunk, chunk, chunk)), TMX, Options{}); err != nil {
		t.Fatalf("four 1000x1000 chunks: %v", err)
	}
	over := tmx(layer(chunk, chunk, chunk, chunk), layer(chunk))
	if _, err := decodeString(over, TMX, Options{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("five 1000x1000 chunks across two layers: %v", err)
	}
	wrap := `<chunk x="0" y="0" width="4611686018427387904" height="2"></chunk>`
	if _, err := decodeString(tmx(layer(wrap, chunk, chunk, chunk, chunk, chunk)), TMX, Options{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a chunk whose area overflows: %v", err)
	}
	if _, err := decodeString(tmx(tmxLayerOf("L", -1, 10, "")), TMX, Options{}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("a negative width: %v", err)
	}
	if _, err := decodeString(tmx(layer(chunk)), TMX, Options{MaxTiles: 999_999}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("MaxTiles not honoured: %v", err)
	}
}

// Every encoding is held to the layer's declared size, including the ones whose data was never inflated: the memory
// of a decode must not depend on how large a file the caller read.
func TestDecodeMapHoldsEveryEncodingToTheDeclaredSize(t *testing.T) {
	t.Parallel()
	many := strings.Repeat("1,", 5000)
	tiles := strings.Repeat(`<tile gid="1"/>`, 5000)
	raw := make([]uint32, 5000)
	for i := range raw {
		raw[i] = 1
	}
	array := "[" + many + "1]"
	for name, c := range map[string]struct {
		doc string
		f   Format
	}{
		"tmx csv":        {tmx(tmxLayerOf("L", 2, 2, `<data encoding="csv">`+many+`</data>`)), TMX},
		"tmx tiles":      {tmx(tmxLayerOf("L", 2, 2, `<data>`+tiles+`</data>`)), TMX},
		"tmx base64":     {tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64">`+compressed(t, "", gids(raw...))+`</data>`)), TMX},
		"tmx chunk csv":  {tmx(`<layer name="L" width="0" height="0"><data encoding="csv"><chunk x="0" y="0" width="2" height="2">` + many + `</chunk></data></layer>`), TMX},
		"tmx height 0":   {tmx(tmxLayerOf("Z", 2, 0, `<data encoding="csv">1,1</data>`)), TMX},
		"tmj array":      {`{"width":2,"height":2,"layers":[{"name":"L","width":2,"height":2,"data":` + array + `}]}`, TMJ},
		"tmj chunk":      {`{"layers":[{"name":"L","chunks":[{"x":0,"y":0,"width":2,"height":2,"data":` + array + `}]}]}`, TMJ},
		"tmj base64":     {`{"layers":[{"name":"L","width":2,"height":2,"data":"` + compressed(t, "", gids(raw...)) + `"}]}`, TMJ},
		"tmj height 0":   {`{"width":2,"layers":[{"name":"L","width":2,"height":0,"data":[1,1]}]}`, TMJ},
		"tmj no height":  {`{"width":2,"layers":[{"name":"L","data":[1]}]}`, TMJ},
		"tmx no size":    {tmx(`<layer name="L"><data encoding="csv">1</data></layer>`), TMX},
		"tmj layer wide": {`{"width":2,"layers":[{"name":"L","height":1,"data":[1,1,1]}]}`, TMJ},
	} {
		if _, err := decodeString(c.doc, c.f, Options{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}

func TestDecodeMapRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	zstd := tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64" compression="zstd">AAAA</data>`))
	if _, err := decodeString(zstd, TMX, Options{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("zstd: %v", err)
	}
	if _, err := decodeString(tmx(tmxLayerOf("L", 2, 2, `<data encoding="ascii">1</data>`)), TMX, Options{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown encoding: %v", err)
	}
	if _, err := decodeString("{}", Format(9), Options{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown format: %v", err)
	}
	for name, c := range map[string]struct {
		doc string
		f   Format
	}{
		"xml cut short":   {`<map width="2"><layer`, TMX},
		"not a map":       {`<other width="2" height="2"/>`, TMX},
		"bad csv":         {tmx(tmxLayerOf("L", 2, 2, `<data encoding="csv">1,x</data>`)), TMX},
		"bad base64":      {tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64">!!</data>`)), TMX},
		"ragged tiles":    {tmx(tmxLayerOf("L", 2, 2, `<data encoding="base64">`+compressed(t, "", []byte{1, 2, 3})+`</data>`)), TMX},
		"json cut short":  {`{"layers":[`, TMJ},
		"data is a count": {`{"layers":[{"name":"L","width":1,"height":1,"data":7}]}`, TMJ},
		"chunk offset":    {`{"layers":[{"name":"L","chunks":[{"x":9223372036854775807,"y":0,"width":2,"height":1,"data":[1,1]}]}]}`, TMJ},
	} {
		if _, err := decodeString(c.doc, c.f, Options{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}

func TestMapSizeReadsTheRootElement(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		doc  string
		f    Format
		w, h int
		ok   bool
	}{
		"tmx":                {`<?xml version="1.0"?><map version="1.10" width="30" height="20"><layer/></map>`, TMX, 30, 20, true},
		"height first":       {`<map height="20" tilewidth="16" width="30"/>`, TMX, 30, 20, true},
		"long head":          {`<!-- ` + strings.Repeat("x", 8000) + ` --><map width="3" height="4"/>`, TMX, 3, 4, true},
		"layer size ignored": {`<map width="3" height="4"><layer width="9" height="9"/></map>`, TMX, 3, 4, true},
		"tmx zero":           {`<map width="0" height="4"/>`, TMX, 0, 0, false},
		"tmx missing":        {`<map width="3"/>`, TMX, 0, 0, false},
		"tmx not a number":   {`<map width="x" height="4"/>`, TMX, 0, 0, false},
		"tmx other root":     {`<tileset width="3" height="4"/>`, TMX, 0, 0, false},
		"tmx empty":          {``, TMX, 0, 0, false},
		"tmj":                {`{"layers":[{"width":9,"height":9}],"width":30,"height":20}`, TMJ, 30, 20, true},
		"tmj negative":       {`{"width":-3,"height":4}`, TMJ, 0, 0, false},
		"tmj not json":       {`<map/>`, TMJ, 0, 0, false},
	} {
		w, h, err := MapSize(strings.NewReader(c.doc), c.f)
		if c.ok != (err == nil) || w != c.w || h != c.h || (!c.ok && !errors.Is(err, ErrMalformed)) {
			t.Errorf("%s: %d x %d, %v", name, w, h, err)
		}
	}
	if _, _, err := MapSize(strings.NewReader("{}"), Format(0)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown format: %v", err)
	}
}
