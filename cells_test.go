package tilemask

import (
	"image"
	"slices"
	"testing"
)

func patternMask(w, h int) *Mask {
	m := newMask(w, h)
	for y := range h {
		for x := range w {
			if (x*7+y*11)%13 < 5 {
				m.set(y*w + x)
			}
		}
	}
	return m
}

func bruteCells(m *Mask, src image.Rectangle, dst image.Point, cell int) []image.Point {
	origin := src.Min
	src = src.Intersect(image.Rect(0, 0, m.Width, m.Height))
	set := map[image.Point]bool{}
	for y := src.Min.Y; y < src.Max.Y; y++ {
		for x := src.Min.X; x < src.Max.X; x++ {
			if m.At(x, y) {
				set[image.Pt(floorDiv(dst.X+x-origin.X, cell), floorDiv(dst.Y+y-origin.Y, cell))] = true
			}
		}
	}
	out := make([]image.Point, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b image.Point) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	return out
}

func TestCellsMatchBruteForce(t *testing.T) {
	t.Parallel()
	m := patternMask(48, 40)
	for _, src := range []image.Rectangle{image.Rect(3, 5, 40, 34), image.Rect(-4, -9, 30, 25)} {
		for _, cell := range []int{1, 16, 32} {
			for _, dst := range []image.Point{{7, 9}, {0, 0}, {-5, -20}, {-16, 3}, {-1, -1}} {
				got := slices.Collect(m.Cells(src, dst, cell))
				if want := bruteCells(m, src, dst, cell); !slices.Equal(got, want) {
					t.Errorf("cell %d dst %v:\ngot  %v\nwant %v", cell, dst, got, want)
				}
			}
		}
	}
}

// Truncating division would put pixels at -15..-1 in cell 0 beside 0..15.
func TestCellsFloorNegativeDestination(t *testing.T) {
	t.Parallel()
	m := newMask(16, 1)
	m.set(0)
	got := slices.Collect(m.Cells(image.Rect(0, 0, 16, 1), image.Pt(-15, 0), 16))
	if want := []image.Point{{-1, 0}}; !slices.Equal(got, want) {
		t.Fatalf("cells = %v, want %v", got, want)
	}
	m.set(15)
	got = slices.Collect(m.Cells(image.Rect(0, 0, 16, 1), image.Pt(-15, 0), 16))
	if want := []image.Point{{-1, 0}, {0, 0}}; !slices.Equal(got, want) {
		t.Fatalf("cells = %v, want %v", got, want)
	}
}

func TestCellsEmptyCases(t *testing.T) {
	t.Parallel()
	m := patternMask(20, 20)
	full := image.Rect(0, 0, 20, 20)
	for name, seq := range map[string][]image.Point{
		"cell 0":         slices.Collect(m.Cells(full, image.Point{}, 0)),
		"negative cell":  slices.Collect(m.Cells(full, image.Point{}, -4)),
		"negative width": slices.Collect(m.Cells(image.Rectangle{Min: image.Pt(5, 5), Max: image.Pt(2, 9)}, image.Point{}, 16)),
		"outside":        slices.Collect(m.Cells(image.Rect(30, 30, 40, 40), image.Point{}, 16)),
		"huge dst":       slices.Collect(m.Cells(full, image.Pt(1<<62, 0), 16)),
		"huge src":       slices.Collect(m.Cells(image.Rect(-1<<62, 0, 1<<62, 20), image.Point{}, 16)),
	} {
		if len(seq) != 0 {
			t.Errorf("%s yielded %v", name, seq)
		}
	}
}

func TestCellsStopsWhenYieldReturnsFalse(t *testing.T) {
	t.Parallel()
	n := 0
	for range patternMask(48, 40).Cells(image.Rect(0, 0, 48, 40), image.Point{}, 4) {
		n++
		break
	}
	if n != 1 {
		t.Fatalf("n = %d", n)
	}
}
