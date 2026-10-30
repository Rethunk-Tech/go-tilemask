# HUMANS.md

Run and use `go-tilemask`. Internals: [AGENTS.md](AGENTS.md).

## Quick start

```bash
go get github.com/Rethunk-Tech/go-tilemask
```

```go
package main

import (
	"fmt"
	"image"
	"os"

	"github.com/Rethunk-Tech/go-tilemask"
)

func main() {
	f, err := os.Open("sheet.png")
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	defer func() { _ = f.Close() }()

	// The reader is read to the end: bound it (io.LimitReader) before passing it in.
	mask, err := tilemask.DecodePNG(f, tilemask.Options{})
	if err != nil {
		fmt.Println("decode:", err) // errors.Is: ErrTooLarge, ErrUnsupported, ErrMalformed
		return
	}
	// Which 16x16 cells does the whole sheet touch, placed at (32, 0)?
	whole := image.Rect(0, 0, mask.Width, mask.Height)
	for cell := range mask.Cells(whole, image.Pt(32, 0), 16) {
		fmt.Println(cell.X, cell.Y)
	}
}
```

## Contracts

| Behaviour | Contract |
| --- | --- |
| `DecodePNG` | Reads the reader to the end. Refuses a claimed size over `MaxPixels` (`ErrTooLarge`) before allocating a pixel buffer, at any bit depth. The fast path (8-bit, non-interlaced) checks no CRC or Adler-32, needs no `PLTE`, and needs no `IDAT` for an opaque grey or RGB image, each erring toward more coverage; an image that is not a PNG or cannot be decoded is `ErrMalformed`. |
| `Mask.At` | False outside the bounds. Safe for concurrent reads. |
| `Mask.Cells` | Row-major, each cell once, floor division. Yields nothing for `cell <= 0`, an empty clipped `src`, or a coordinate beyond a quarter of the int range. Build `src` as `image.Rectangle{Min, Max}`, not `image.Rect`, which swaps a negative extent into a positive one. |
| `Mask.WriteTo` | Writes gzip of `W H\n` and the packed bits, least-significant bit first; refuses a mask with a non-positive size, which `ReadMask` would refuse. Never touches a path. |
| `ReadMask` | Refuses a header over `MaxPixels` before allocating; a torn or flipped file fails the gzip trailer (`ErrMalformed`). Cap it with the same `MaxPixels` the mask was decoded under so every mask written reads back. |
| `DecodeMap` | Top-level tile layers only, in document order; layer names verbatim. Occupied is GID not zero, flip bits included. Declared sizes of all layers and chunks together must fit `MaxTiles` (`ErrTooLarge`) before any data inflates; a layer or chunk with more values than width times height is `ErrMalformed`. zstd and unknown encodings are `ErrUnsupported` and fail the whole map. |
| `MapSize` | Header only: TMX stops at the root `<map>` element whatever the attribute order; TMJ reads the top-level object. A size that is missing or not positive is `ErrMalformed`. |
| Options | A zero field means its default (`DefaultMaxPixels`, `DefaultMaxTiles`). The input's byte size is the caller's to bound. |

## Requirements

Go 1.27.3 or newer, plus `lefthook` and `gitleaks` on PATH if you want the git hooks.

```bash
lefthook install
```

That is per clone; git does not commit hooks. Pre-commit scans staged changes for secrets, and
pre-push runs `gate`.

## Verify

```bash
gate
```
