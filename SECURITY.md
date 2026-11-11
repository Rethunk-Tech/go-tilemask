# Security

## Supported versions

The latest tagged release. There are no long-lived release branches, so a fix ships in the next tag
rather than being backported.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository: <https://github.com/Rethunk-Tech/go-tilemask/security/advisories/new>, or the **Security** tab, then **Report a vulnerability**. The report stays private between you and us until a fix ships.

If private reporting is unavailable, email [oss@rethunk.tech](mailto:oss@rethunk.tech) with **[SECURITY]** in the subject.

Please do not open a public issue, pull request or discussion for a suspected vulnerability.

A useful report has the affected repository and version (or commit), steps to reproduce, what an attacker gets and under which conditions, and whether anyone else knows.

## What counts as a vulnerability

A vulnerability is a flaw that lets someone cross a boundary they should not: run code, read or change data they have no right to, escape a sandbox or path restriction, or leak a credential. A bug is anything else that misbehaves, such as a crash on your own input, wrong output or a slow path; open an ordinary issue for those. If you are unsure which one you have, report it privately and we will say.

## Coordinated disclosure

We handle the report inside the private advisory, so you see each status change and can comment on the fix. The advisory goes public when the fix ships, or 90 days after your report, whichever comes first. If a fix is close, either of us can ask for more time in the thread. You are credited in the advisory unless you would rather not be.

## Response timelines

- We acknowledge your report within 3 business days.
- We give you an assessment (accepted, needs more information or declined, with reasons) within 10 business days.
- We aim to ship a fix within 90 days of the report.

## What is in scope

- `DecodePNG`, a PNG of any size or shape: a claimed size over `MaxPixels` (half of it for 16-bit colour, which `image/png` holds at 8 bytes a pixel) must be refused before any pixel buffer is allocated, with no wrapping product letting one pass, and it must not panic or read past the input.
- `ReadMask`, a mask file: the header line is read under a 64-byte limit and its size held to `MaxPixels` before its bits are allocated, and a damaged file must fail rather than return a wrong mask.
- `DecodeMap` and `MapSize`, a TMX or TMJ document: TMX element nesting is held to 64 levels, the declared tiles of the whole map must be held to `MaxTiles` before any data inflates, no layer may decode more values than it declared, and chunk offsets must not wrap a tile coordinate.
- `Mask.Cells`, caller-chosen regions and offsets: the working set is bounded by the mask, and no coordinate sum may wrap.

## What is out of scope

- The byte size of every reader is the caller's to bound (`io.LimitReader`); memory is proportional to it, plus what the caps allow.
- Paths, containment and atomic replacement of a stored mask belong to the caller: this package only reads and writes the streams it is given.
- Not a PNG validator: the fast path skips CRCs and the Adler-32 trailer by design, always toward more coverage. A caller that needs a valid file runs `png.DecodeConfig` itself.
- XML entities and DTDs are not expanded: `encoding/xml` leaves them unresolved.

## Safe harbour

We will not pursue legal action against good-faith research that avoids privacy violations, data destruction and disruption of services, stops at a proof of concept, and keeps the details private until the advisory is published. Rethunk.Tech, LLC maintains this repository.
