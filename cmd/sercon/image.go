package main

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	// Register decoders for image.Decode sniffing.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// encodeOpts carries optional encode knobs (quality for jpeg/webp).
type encodeOpts struct{ quality int }

// decodeImage sniffs and decodes data into an image, returning the detected
// format name. Decoders for png/jpeg/gif (stdlib) and webp/tiff/bmp (x/image,
// blank-imported) are registered, so image.Decode works by magic bytes.
func decodeImage(data []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("image.decode: unsupported or corrupt image: %w", err)
	}
	return img, format, nil
}

// encodeImage encodes img to the named format. quality (1..100) applies to
// jpeg/webp; png/gif/tiff/bmp ignore it.
func encodeImage(img image.Image, format string, o encodeOpts) ([]byte, error) {
	q := o.quality
	if q <= 0 || q > 100 {
		q = 90
	}
	var buf bytes.Buffer
	switch strings.ToLower(format) {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
	case "gif":
		if err := gif.Encode(&buf, img, nil); err != nil {
			return nil, err
		}
	case "tiff", "tif":
		if err := tiff.Encode(&buf, img, nil); err != nil {
			return nil, err
		}
	case "bmp":
		if err := bmp.Encode(&buf, img); err != nil {
			return nil, err
		}
	case "webp":
		b, err := encodeWebP(img, q)
		if err != nil {
			return nil, err
		}
		return b, nil
	default:
		return nil, fmt.Errorf("image: unsupported encode format %q (supported: png, jpeg, gif, tiff, bmp, webp)", format)
	}
	return buf.Bytes(), nil
}

// encodeWebP encodes img as WebP via nativewebp (pure-Go, lossless — quality is
// accepted for API symmetry but the encoder is lossless, so it is ignored).
func encodeWebP(img image.Image, _ int) ([]byte, error) {
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, img, nil); err != nil {
		return nil, fmt.Errorf("image: webp encode: %w", err)
	}
	return buf.Bytes(), nil
}

// inferFormatFromPath maps a file extension to an encode format name.
func inferFormatFromPath(path string) (string, error) {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "png":
		return "png", nil
	case "jpg", "jpeg":
		return "jpeg", nil
	case "gif":
		return "gif", nil
	case "tif", "tiff":
		return "tiff", nil
	case "bmp":
		return "bmp", nil
	case "webp":
		return "webp", nil
	default:
		return "", fmt.Errorf("image: cannot infer format from path %q", path)
	}
}
