// cmd/sercon/exif_imagemeta.go
package main

import (
	"bytes"
	"math"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/meta/exif"
)

// readExifImagemeta reads the curated EXIF field set imagemeta exposes for
// formats dsoprea cannot handle (HEIC/AVIF/RAW/CR2/NEF/ARW), mapped into the
// same grouped exifDoc shape. Only the documented curated subset is populated
// (partial coverage by design — partial keys match what dsoprea would produce).
func readExifImagemeta(data []byte) (exifDoc, error) {
	ex, err := imagemeta.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return imagemetaToDoc(ex), nil
}

// imagemetaToDoc maps imagemeta's typed fields into the canonical grouped model.
//
// Field sources:
//   - image group  ← ex.IFD0.Make / Model / Orientation
//   - exif group   ← ex.ExifIFD.DateTimeOriginal / ExposureTime / ISOSpeedRatings /
//     FNumber / FocalLength / LensModel
//   - gps group    ← ex.GPS.Latitude() / Longitude() (signed decimal degrees)
//
// Only non-zero / non-empty values are written. Empty groups are pruned before
// returning so callers see the same sparse-but-consistent shape as the dsoprea backend.
func imagemetaToDoc(ex exif.Exif) exifDoc {
	doc := exifDoc{
		"image": {},
		"exif":  {},
		"gps":   {},
	}

	// ── image group ──────────────────────────────────────────────────────────
	if ex.IFD0.Make != "" {
		doc["image"]["Make"] = ex.IFD0.Make
	}
	if ex.IFD0.Model != "" {
		doc["image"]["Model"] = ex.IFD0.Model
	}
	if ex.IFD0.Orientation != 0 {
		doc["image"]["Orientation"] = int(ex.IFD0.Orientation)
	}

	// ── exif group ───────────────────────────────────────────────────────────
	dto := ex.ExifIFD.DateTimeOriginal
	if !dto.IsZero() {
		doc["exif"]["DateTimeOriginal"] = dto.Format("2006:01:02 15:04:05")
	}
	// ExposureTime / FNumber / FocalLength are rational EXIF tags that the dsoprea
	// read path emits as [num,den] arrays. imagemeta pre-divides them into floats,
	// so the exact original numerator/denominator aren't recoverable — we rebuild a
	// faithful [num,den] approximation so the value and the array shape match dsoprea.
	if r := secsToRational(float32(ex.ExifIFD.ExposureTime)); r != nil {
		doc["exif"]["ExposureTime"] = r
	}
	if ex.ExifIFD.ISOSpeedRatings != 0 {
		doc["exif"]["ISOSpeedRatings"] = ex.ExifIFD.ISOSpeedRatings
	}
	if r := decimalToRational(float32(ex.ExifIFD.FNumber)); r != nil {
		doc["exif"]["FNumber"] = r
	}
	if r := decimalToRational(float32(ex.ExifIFD.FocalLength)); r != nil {
		doc["exif"]["FocalLength"] = r
	}
	if ex.ExifIFD.LensModel != "" {
		doc["exif"]["LensModel"] = ex.ExifIFD.LensModel
	}

	// ── gps group ────────────────────────────────────────────────────────────
	lat := ex.GPS.Latitude()
	lon := ex.GPS.Longitude()
	if lat != 0 || lon != 0 {
		doc["gps"]["GPSLatitude"] = lat
		doc["gps"]["GPSLongitude"] = lon
	}

	pruneEmptyGroups(doc)
	return doc
}

// secsToRational expresses a shutter speed in seconds as the EXIF rational
// form cameras use: sub-second speeds as 1/N, otherwise N/1. Returns nil for
// non-positive input so the caller can omit the field.
func secsToRational(v float32) []any {
	if v <= 0 {
		return nil
	}
	if v < 1 {
		return []any{uint32(1), uint32(math.Round(1 / float64(v)))}
	}
	return []any{uint32(math.Round(float64(v))), uint32(1)}
}

// decimalToRational expresses a positive decimal (e.g. f-number, focal length)
// as a hundredths rational [round(v*100), 100], matching the [num,den] shape
// the dsoprea path emits. Returns nil for non-positive input.
func decimalToRational(v float32) []any {
	if v <= 0 {
		return nil
	}
	return []any{uint32(math.Round(float64(v) * 100)), uint32(100)}
}

// pruneEmptyGroups removes any group key whose map has no entries.
func pruneEmptyGroups(doc exifDoc) {
	for g, m := range doc {
		if len(m) == 0 {
			delete(doc, g)
		}
	}
}
