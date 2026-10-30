# AGENTS.md

Package `tilemask` reports which pixels of a PNG, and which tiles of a Tiled map, are occupied, and projects a pixel mask onto a grid of any cell size. Any tool that reads sprite art or TMX/TMJ maps imports it. It imports only the standard library, no file-system module, and never names a game, format owner or caller.

## Gate

`gate` is the command. Each override in `.gate.toml` is there because a runner
cannot resolve that check from PATH.

## Shape

- Every size check goes through `areaWithin`, which divides rather than multiplies: a declared size whose product wraps negative would otherwise pass.
- A cap is checked before the allocation it guards: `MaxPixels` before any pixel buffer or mask bits, `MaxTiles` over the whole map before any data inflates.
- The tile budget is the declared size of each layer or chunk: a block decodes at most `width x height` values and more is `ErrMalformed`, so decode memory does not follow the caller's byte cap.
- Error classes: `ErrTooLarge` a cap, `ErrUnsupported` a well-formed input this package does not read, `ErrMalformed` everything else. Callers branch on these, so a new failure picks one.
- The PNG fast path and `image/png` must agree whenever both accept an input (`FuzzDecodePNG`). The fast path skips checksums and needs no `PLTE`/`IDAT` in some cases, always toward more coverage; do not add validation there, since it exists to skip that work.
- Padding bits of a `Mask` stay zero, so equal masks hold equal bytes and the file format is reproducible.
- The mask file format is a stored contract: `testdata/mask-gzip-19x5.gz` is read and rewritten byte for byte. Change the format only with a reader for the old one.
- `Cells` floors, so negative offsets land in negative cells; `src` is placed by its unclipped top-left.
- No pool and no release call: masks are ordinary memory, because a recycled mask is a use-after-release hazard for every reader still holding one.
- Layers are top-level only and names verbatim: grouping and case folding are the caller's rules.
- Nothing opens a path. Files, containment and atomic writes are the caller's.

## Files

| File | Holds |
| --- | --- |
| `doc.go` | Package documentation |
| `bounds.go` | `Options`, default caps, the error values, `areaWithin` |
| `mask.go` | `Mask`, `At`, `Cells` |
| `maskio.go` | `WriteTo`, `ReadMask`: the gzip mask file format |
| `png.go` | `DecodePNG`: the fast path, its unfilter, and the `image/png` fallback |
| `tiled.go` | `Map`, `Layer`, `Format`, `DecodeMap`, `MapSize`, the declared-size check |
| `tiledvalues.go` | TMX and TMJ document parsing and the CSV/base64/zlib/gzip value decoder |
