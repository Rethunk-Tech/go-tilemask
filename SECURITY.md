# Security

## Supported versions

The latest tagged release. There are no long-lived release branches, so a fix ships in the next tag
rather than being backported.

## Reporting a vulnerability

Use GitHub's **private vulnerability reporting** on this repository (Security → Report a
vulnerability). It stays private until a fix ships.

Please do not open a public issue for a suspected vulnerability first.

## What is in scope

- `DecodePNG`, a PNG of any size or shape: a claimed size over `MaxPixels` must be refused before any pixel buffer is allocated, with no wrapping product letting one pass, and it must not panic or read past the input.
- `ReadMask`, a mask file: the header's size must be held to `MaxPixels` before its bits are allocated, and a damaged file must fail rather than return a wrong mask.
- `DecodeMap` and `MapSize`, a TMX or TMJ document: the declared tiles of the whole map must be held to `MaxTiles` before any data inflates, no layer may decode more values than it declared, and chunk offsets must not wrap a tile coordinate.
- `Mask.Cells`, caller-chosen regions and offsets: the working set is bounded by the mask, and no coordinate sum may wrap.

## What is out of scope

- The byte size of every reader is the caller's to bound (`io.LimitReader`); memory is proportional to it, plus what the caps allow.
- Paths, containment and atomic replacement of a stored mask belong to the caller: this package only reads and writes the streams it is given.
- Not a PNG validator: the fast path skips CRCs and the Adler-32 trailer by design, always toward more coverage. A caller that needs a valid file runs `png.DecodeConfig` itself.
- XML entities and DTDs are not expanded: `encoding/xml` leaves them unresolved.
