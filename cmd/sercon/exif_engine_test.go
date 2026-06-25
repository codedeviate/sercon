// cmd/sercon/exif_engine_test.go
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func plainJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func plainPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestExifJPEG_RoundTrip_Replace(t *testing.T) {
	src := plainJPEG(t)
	doc := exifDoc{
		"image": {"Make": "Canon", "Model": "EOS R5", "Orientation": float64(1)},
		"exif":  {"ExposureTime": []any{float64(1), float64(250)}},
	}
	out, err := writeExifJPEG(src, doc, modeReplace)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := extractRawExif(out, "jpeg")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := readExifDsoprea(raw)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got["image"]["Make"] != "Canon" || got["image"]["Model"] != "EOS R5" {
		t.Fatalf("round-trip lost image tags: %#v", got["image"])
	}
}

func TestExifJPEG_Merge_NullDeletes(t *testing.T) {
	src := plainJPEG(t)
	// base has Copyright, which the merge doc never mentions — it must survive.
	base, _ := writeExifJPEG(src, exifDoc{"image": {"Make": "Canon", "Artist": "X", "Copyright": "C"}}, modeReplace)
	// merge: update Make, delete Artist (null), add Model, leave Copyright alone
	merged, err := writeExifJPEG(base, exifDoc{"image": {"Make": "Nikon", "Artist": nil, "Model": "Z9"}}, modeMerge)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	raw, _ := extractRawExif(merged, "jpeg")
	got, _ := readExifDsoprea(raw)
	if got["image"]["Make"] != "Nikon" || got["image"]["Model"] != "Z9" {
		t.Fatalf("merge update/add failed: %#v", got["image"])
	}
	if _, present := got["image"]["Artist"]; present {
		t.Fatalf("merge null-delete failed: Artist still present")
	}
	if got["image"]["Copyright"] != "C" {
		t.Fatalf("merge dropped an untouched tag: Copyright=%#v (want \"C\")", got["image"]["Copyright"])
	}
}

func TestExifPNG_RoundTrip(t *testing.T) {
	out, err := writeExifPNG(plainPNG(t), exifDoc{"image": {"Make": "Sony"}}, modeReplace)
	if err != nil {
		t.Fatalf("png write: %v", err)
	}
	raw, err := extractRawExif(out, "png")
	if err != nil {
		t.Fatalf("png extract: %v", err)
	}
	got, _ := readExifDsoprea(raw)
	if got["image"]["Make"] != "Sony" {
		t.Fatalf("png round-trip: %#v", got["image"])
	}
}

func TestExifClear(t *testing.T) {
	with, _ := writeExifJPEG(plainJPEG(t), exifDoc{"image": {"Make": "Canon"}}, modeReplace)
	cleared, err := writeExifJPEG(with, nil, modeClear)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := extractRawExif(cleared, "jpeg"); err != errNoExif {
		t.Fatalf("clear should remove EXIF, got err=%v", err)
	}
}

func TestExifClear_PNG(t *testing.T) {
	with, err := writeExifPNG(plainPNG(t), exifDoc{"image": {"Make": "Sony"}}, modeReplace)
	if err != nil {
		t.Fatalf("png write: %v", err)
	}
	// sanity: EXIF really is present before clearing
	if _, err := extractRawExif(with, "png"); err != nil {
		t.Fatalf("png precondition: expected EXIF present, got err=%v", err)
	}
	cleared, err := writeExifPNG(with, nil, modeClear)
	if err != nil {
		t.Fatalf("png clear: %v", err)
	}
	if _, err := extractRawExif(cleared, "png"); err != errNoExif {
		t.Fatalf("png clear should remove EXIF, got err=%v", err)
	}
}
