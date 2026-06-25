// cmd/sercon/image_orient.go
package main

import (
	"image"

	"github.com/disintegration/imaging"
)

// applyOrientation returns img transformed to be upright for the given EXIF
// Orientation value (1..8). Built from imaging's flips/rotations (imaging
// rotates counter-clockwise for positive angles; Rotate270 == 90° CW).
// n==1 or any unrecognised value returns an upright copy.
func applyOrientation(img image.Image, n int) image.Image {
	switch n {
	case 2:
		return imaging.FlipH(img)
	case 3:
		return imaging.Rotate180(img)
	case 4:
		return imaging.FlipV(img)
	case 5:
		return imaging.Transpose(img)
	case 6:
		return imaging.Rotate270(img) // 90° clockwise
	case 7:
		return imaging.Transverse(img)
	case 8:
		return imaging.Rotate90(img) // 90° counter-clockwise
	default:
		return imaging.Clone(img)
	}
}

// orientationToInt coerces a decoded EXIF Orientation value to an int in 1..8,
// defaulting to 1 for anything missing or out of range.
func orientationToInt(v any) int {
	var n int
	switch t := v.(type) {
	case uint16:
		n = int(t)
	case int:
		n = t
	case int64:
		n = int(t)
	case float64:
		n = int(t)
	default:
		return 1
	}
	if n < 1 || n > 8 {
		return 1
	}
	return n
}

// exifOrientation reads the EXIF Orientation (1..8) from container bytes.
// JPEG/PNG/TIFF use the dsoprea full reader; other formats (HEIC/AVIF/RAW) use
// the imagemeta fallback. Absent/unreadable Orientation returns 1.
func exifOrientation(data []byte, format string) int {
	var doc exifDoc
	switch format {
	case "jpeg", "png", "tiff":
		raw, err := extractRawExif(data, format)
		if err != nil {
			return 1 // includes errNoExif
		}
		doc, err = readExifDsoprea(raw)
		if err != nil {
			return 1
		}
	default:
		var err error
		doc, err = readExifImagemeta(data)
		if err != nil {
			return 1
		}
	}
	if g := doc["image"]; g != nil {
		if v, ok := g["Orientation"]; ok {
			return orientationToInt(v)
		}
	}
	return 1
}
