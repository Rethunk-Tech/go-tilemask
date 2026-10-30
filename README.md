<h1 align="center">go-tilemask</h1>

<div align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.27.3+-00ADD8?logo=go&logoColor=white)](go.mod)

</div>

---

Bounded PNG alpha masks, grid-cell projection and Tiled TMX/TMJ occupied-tile decoding for game art. Package `tilemask`, module `github.com/Rethunk-Tech/go-tilemask`.

Answers one question about game art: which cells of an image, or which tiles of a map, are used. A cell is a square of a size the caller picks. A PNG becomes a one-bit-per-pixel mask (alpha not zero) that projects onto any grid, and a Tiled map becomes the occupied tiles of each top-level layer. Both decoders bound what their input claims (pixels, tiles, inflation) before allocating, so a few hostile bytes cannot ask for gigabytes. The caller bounds the input's byte size. Stdlib only.

## Quick start

```bash
go get github.com/Rethunk-Tech/go-tilemask
```

A worked example and the contracts: [HUMANS.md](HUMANS.md).

## Features

- A pixel is set when its alpha is not zero, at any bit depth; `Mask.At` and `Mask.Cells` read it.
- `Cells` projects any region onto any cell size with floor division, so negative offsets land in negative cells. `cell = 1` costs one byte per pixel while it runs.
- The PNG fast path reads 8-bit, non-interlaced images and checks no CRC or Adler-32, needs no `PLTE`, and needs no `IDAT` for an opaque grey or RGB image; each leniency errs toward more coverage. Everything else goes through `image/png`.
- `DecodeMap` reads TMX and TMJ under one tile cap for the whole map, declared and decoded, and refuses a layer or chunk that holds more values than it declared.
- `WriteTo` and `ReadMask` store a mask as gzip of `W H\n` and the packed bits, least-significant bit first; the reader holds the header to the pixel cap and verifies the gzip trailer.
- Errors say why: `ErrTooLarge`, `ErrUnsupported` (zstd, unknown encodings) or `ErrMalformed`.

## Documentation

| Doc | Audience |
| --- | --- |
| [HUMANS.md](HUMANS.md) | Running and using the package |
| [AGENTS.md](AGENTS.md) | Internals, invariants, and why |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Process — commits, tests, docs |
| [SECURITY.md](SECURITY.md) | Vulnerability disclosure |
| [CHANGELOG.md](CHANGELOG.md) | Release notes |

## License

MIT — see [LICENSE](LICENSE). © 2026 Rethunk.Tech, LLC.
