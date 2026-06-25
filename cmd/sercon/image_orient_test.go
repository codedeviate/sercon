// cmd/sercon/image_orient_test.go
package main

import (
	"image"
	"image/color"
	"testing"
)

// markerImg builds a 3x2 image with a unique red top-left pixel and white
// elsewhere, so transforms are detectable by where red lands and by dims.
func markerImg() *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			m.Set(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	m.Set(0, 0, color.NRGBA{255, 0, 0, 255}) // red marker at top-left
	return m
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0x8000 && g < 0x4000 && b < 0x4000
}

func TestApplyOrientation_Dimensions(t *testing.T) {
	src := markerImg() // 3x2
	// 1-4 preserve W x H; 5-8 swap to H x W.
	for _, n := range []int{1, 2, 3, 4} {
		b := applyOrientation(src, n).Bounds()
		if b.Dx() != 3 || b.Dy() != 2 {
			t.Fatalf("orient(%d) dims = %dx%d, want 3x2", n, b.Dx(), b.Dy())
		}
	}
	for _, n := range []int{5, 6, 7, 8} {
		b := applyOrientation(src, n).Bounds()
		if b.Dx() != 2 || b.Dy() != 3 {
			t.Fatalf("orient(%d) dims = %dx%d, want 2x3", n, b.Dx(), b.Dy())
		}
	}
}

func TestApplyOrientation_Identity(t *testing.T) {
	src := markerImg()
	out := applyOrientation(src, 1)
	if !isRed(out.At(0, 0)) {
		t.Fatal("orient(1) should keep red at top-left")
	}
}

func TestApplyOrientation_FlipH(t *testing.T) {
	// orientation 2 = mirror horizontal: red top-left → top-right (x=2).
	out := applyOrientation(markerImg(), 2)
	if !isRed(out.At(2, 0)) {
		t.Fatal("orient(2) should move red marker to top-right")
	}
	if isRed(out.At(0, 0)) {
		t.Fatal("orient(2) should not leave red at top-left")
	}
}

func TestApplyOrientation_6and8Differ(t *testing.T) {
	// 6 (90° CW) and 8 (90° CCW) must not produce the same marker position.
	a := applyOrientation(markerImg(), 6)
	b := applyOrientation(markerImg(), 8)
	redAt := func(im image.Image) (int, int) {
		bb := im.Bounds()
		for y := bb.Min.Y; y < bb.Max.Y; y++ {
			for x := bb.Min.X; x < bb.Max.X; x++ {
				if isRed(im.At(x, y)) {
					return x, y
				}
			}
		}
		return -1, -1
	}
	ax, ay := redAt(a)
	bx, by := redAt(b)
	if ax == bx && ay == by {
		t.Fatalf("orient(6) and orient(8) put red at the same spot (%d,%d)", ax, ay)
	}
}

func TestExifOrientation_RoundTrip(t *testing.T) {
	// Write a known Orientation into a JPEG via the shipped EXIF writer,
	// then read it back through exifOrientation.
	src := plainJPEG(t)
	out, err := writeExifJPEG(src, exifDoc{"image": {"Orientation": int64(6)}}, modeReplace)
	if err != nil {
		t.Fatalf("writeExifJPEG: %v", err)
	}
	if got := exifOrientation(out, "jpeg"); got != 6 {
		t.Fatalf("exifOrientation = %d, want 6", got)
	}
	// A plain JPEG with no EXIF → defaults to 1.
	if got := exifOrientation(src, "jpeg"); got != 1 {
		t.Fatalf("exifOrientation(no-exif) = %d, want 1", got)
	}
}
