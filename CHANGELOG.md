# Changelog

Notable changes to `go-tilemask`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `DecodePNG`: a bounded PNG decoder that returns a one-bit-per-pixel `Mask` of pixels with alpha not zero, with the pixel cap checked before any buffer is allocated.
- `Mask.Cells`: projects a mask region onto a grid of any cell size, row-major with floor division, so cells left of or above the origin are negative.
- `Mask.WriteTo` and `ReadMask`: a gzip mask file format whose reader holds the header to the pixel cap before allocating and verifies the gzip trailer.
- `DecodeMap`: reads Tiled TMX and TMJ maps (CSV, tile lists, base64 with none, zlib or gzip) into each top-level tile layer's occupied tiles, under one tile cap for the whole map, with no layer or chunk allowed more values than it declared.
- `MapSize`: reads a map's size from its root element or top-level object without decoding any layer.

[Unreleased]: https://github.com/Rethunk-Tech/go-tilemask/commits/main
