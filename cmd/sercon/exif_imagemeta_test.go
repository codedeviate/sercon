// cmd/sercon/exif_imagemeta_test.go
package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/evanoberholster/imagemeta/meta"
	"github.com/evanoberholster/imagemeta/meta/exif"
)

func TestImagemetaToDoc(t *testing.T) {
	// Build a minimal exif.Exif value using the real struct fields.
	// IFD0 holds Make/Model/Orientation; ExifIFD holds the photographic fields.
	ex := exif.Exif{}
	ex.IFD0.Make = "Apple"
	ex.IFD0.Model = "iPhone 15"
	ex.IFD0.Orientation = meta.Orientation(6)

	doc := imagemetaToDoc(ex)

	if doc["image"]["Make"] != "Apple" || doc["image"]["Model"] != "iPhone 15" {
		t.Fatalf("make/model mapping: %#v", doc["image"])
	}
	if doc["image"]["Orientation"] != 6 {
		t.Fatalf("orientation mapping: got %v, want 6", doc["image"]["Orientation"])
	}
}

func TestImagemetaToDoc_ExifFields(t *testing.T) {
	ex := exif.Exif{}
	ex.ExifIFD.ISOSpeedRatings = 400
	ex.ExifIFD.FNumber = meta.Aperture(2.8)
	ex.ExifIFD.FocalLength = meta.FocalLength(35)
	ex.ExifIFD.ExposureTime = meta.ExposureTime(0.01)
	ex.ExifIFD.LensModel = "iPhone 15 back camera 6.86mm f/1.78"

	t0, _ := time.Parse("2006:01:02 15:04:05", "2024:06:01 10:30:00")
	ex.ExifIFD.DateTimeOriginal = t0

	doc := imagemetaToDoc(ex)

	if _, ok := doc["exif"]; !ok {
		t.Fatal("exif group missing")
	}
	if doc["exif"]["ISOSpeedRatings"] != uint32(400) {
		t.Errorf("ISO: got %v, want 400", doc["exif"]["ISOSpeedRatings"])
	}
	// Rational EXIF tags must match the dsoprea [num,den] array shape exactly.
	if got := doc["exif"]["ExposureTime"]; !reflect.DeepEqual(got, []any{uint32(1), uint32(100)}) {
		t.Errorf("ExposureTime: got %#v, want [1,100] (1/100s)", got)
	}
	if got := doc["exif"]["FNumber"]; !reflect.DeepEqual(got, []any{uint32(280), uint32(100)}) {
		t.Errorf("FNumber: got %#v, want [280,100] (2.8)", got)
	}
	if got := doc["exif"]["FocalLength"]; !reflect.DeepEqual(got, []any{uint32(3500), uint32(100)}) {
		t.Errorf("FocalLength: got %#v, want [3500,100] (35mm)", got)
	}
	if doc["exif"]["LensModel"] != "iPhone 15 back camera 6.86mm f/1.78" {
		t.Errorf("LensModel: got %v", doc["exif"]["LensModel"])
	}
	if doc["exif"]["DateTimeOriginal"] != "2024:06:01 10:30:00" {
		t.Errorf("DateTimeOriginal: got %v", doc["exif"]["DateTimeOriginal"])
	}
}

func TestImagemetaToDoc_PrunesEmptyGroups(t *testing.T) {
	// An Exif with only Make set: image group present, exif+gps pruned.
	ex := exif.Exif{}
	ex.IFD0.Make = "Canon"

	doc := imagemetaToDoc(ex)
	if _, ok := doc["exif"]; ok {
		t.Error("exif group should be pruned when empty")
	}
	if _, ok := doc["gps"]; ok {
		t.Error("gps group should be pruned when empty")
	}
}
