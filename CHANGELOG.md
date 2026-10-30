# Changelog

Notable changes to `go-tilemask`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `DecodePNG`: a bounded PNG decoder that returns a one-bit-per-pixel `Mask` of pixels with alpha not zero, with the pixel cap checked before any buffer is allocated.
- `Mask.Cells`: projects a mask region onto a grid of any cell size, row-major with floor division, so cells left of or above the origin are negative.

[Unreleased]: https://github.com/Rethunk-Tech/go-tilemask/commits/main
