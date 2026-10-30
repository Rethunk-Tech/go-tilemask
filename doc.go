// Package tilemask answers which cells of an image, or which tiles of a Tiled map, are used.
//
// DecodePNG keeps one bit per pixel (alpha not zero) under a pixel cap checked before any buffer is allocated, and
// Mask.Cells projects that onto a grid of any cell size. DecodeMap lists each top-level tile layer's occupied tiles
// under one tile cap for the whole map. Every decoder bounds what its input claims, not the input's byte size: the
// caller limits the reader.
package tilemask
