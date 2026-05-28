package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"github.com/dop251/goja"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/aztec"
	"github.com/makiuchi-d/gozxing/datamatrix"
	"github.com/makiuchi-d/gozxing/oned"
	"github.com/makiuchi-d/gozxing/qrcode"
	_ "golang.org/x/image/webp" // register WebP decoder with image.Decode
)

// barcodeDecodableFormats lists the symbologies api.format.barcode.decode can
// recognise. PDF417 isn't here even though api.format.barcode.encode produces
// it — gozxing v0.1.1 has no PDF417 decoder. Scripts that need to
// round-trip PDF417 will get a clear "decoder not available" error
// from formatReader rather than a silent miss.
var barcodeDecodableFormats = []string{
	"qr", "datamatrix", "aztec",
	"code128", "code39", "code93", "codabar",
	"ean13", "ean8", "upca", "upce", "itf",
}

// formatReader maps our snake-case format name onto a gozxing Reader
// constructor. Returns nil when the name is unknown or unsupported
// (e.g. pdf417); the caller produces a uniform error from that. Kept
// in lockstep with barcodeDecodableFormats and the BarcodeFormat
// strings emitted by gozxingFormatString.
func formatReader(name string) gozxing.Reader {
	switch name {
	case "qr":
		return qrcode.NewQRCodeReader()
	case "datamatrix":
		return datamatrix.NewDataMatrixReader()
	case "aztec":
		return aztec.NewAztecReader()
	case "code128":
		return oned.NewCode128Reader()
	case "code39":
		return oned.NewCode39Reader()
	case "code93":
		return oned.NewCode93Reader()
	case "codabar":
		return oned.NewCodaBarReader()
	case "ean13":
		return oned.NewEAN13Reader()
	case "ean8":
		return oned.NewEAN8Reader()
	case "upca":
		return oned.NewUPCAReader()
	case "upce":
		return oned.NewUPCEReader()
	case "itf":
		return oned.NewITFReader()
	default:
		return nil
	}
}

// gozxingFormatString maps the BarcodeFormat enum back to our
// snake-case label. Driven from the reverse direction of formatReader
// so encode("qr")/decode roundtrips report the same string. Returns
// the empty string for unmapped enums (RSS_14 / RSS_EXPANDED /
// MAXICODE / PDF_417 — none of which sercon emits, but they'd be
// safe to no-op rather than panic).
func gozxingFormatString(f gozxing.BarcodeFormat) string {
	switch f {
	case gozxing.BarcodeFormat_QR_CODE:
		return "qr"
	case gozxing.BarcodeFormat_DATA_MATRIX:
		return "datamatrix"
	case gozxing.BarcodeFormat_AZTEC:
		return "aztec"
	case gozxing.BarcodeFormat_CODE_128:
		return "code128"
	case gozxing.BarcodeFormat_CODE_39:
		return "code39"
	case gozxing.BarcodeFormat_CODE_93:
		return "code93"
	case gozxing.BarcodeFormat_CODABAR:
		return "codabar"
	case gozxing.BarcodeFormat_EAN_13:
		return "ean13"
	case gozxing.BarcodeFormat_EAN_8:
		return "ean8"
	case gozxing.BarcodeFormat_UPC_A:
		return "upca"
	case gozxing.BarcodeFormat_UPC_E:
		return "upce"
	case gozxing.BarcodeFormat_ITF:
		return "itf"
	default:
		return ""
	}
}

// autoDecodeOrder is the format priority list used when the caller
// doesn't hint at the symbology. 2D formats first (lower false-positive
// rate — they're scanned globally rather than row-by-row), then 1D
// formats with the most-constrained checksum (UPC/EAN) before the
// permissive ones (Code 128, Codabar) that can decode noise.
var autoDecodeOrder = []string{
	"qr", "datamatrix", "aztec",
	"ean13", "ean8", "upca",
	"code128", "code39", "code93",
	"itf", "codabar",
}

// decodeImageBytes decodes a PNG/JPEG/WebP byte slice via image.Decode,
// which finds the right decoder by sniffing the magic bytes of every
// format registered in image's package init blocks. PNG and JPEG are
// registered by stdlib; WebP is registered by the blank-import of
// golang.org/x/image/webp at the top of this file.
func decodeImageBytes(data []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// image.Decode's "image: unknown format" is the common case
		// when callers hand in non-image bytes. Rewrap so the prefix
		// names the binding rather than image's package path.
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	return img, format, nil
}

// barcodeDecodeCall is the PromisifyAsync workhorse. Signature:
//
//	decode(data, format?)
//
// `data` is a Uint8Array / ArrayBuffer / string containing the image
// bytes (PNG / JPEG / WebP). `format` is the optional symbology hint.
// With a hint, only that reader runs. Without one, every reader in
// autoDecodeOrder is tried in priority order; the first successful
// decode wins. Returns `{ format, text }`. Throws on transport
// errors (bad image bytes, no barcode found).
func barcodeDecodeCall(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("barcode.decode: image data argument required")
	}
	data, err := jsArgToBytes(call.Argument(0))
	if err != nil {
		return nil, fmt.Errorf("barcode.decode: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("barcode.decode: image data is empty")
	}

	formatHint := ""
	if len(call.Arguments) >= 2 {
		hint := call.Argument(1)
		if hint != nil && !goja.IsUndefined(hint) && !goja.IsNull(hint) {
			formatHint = strings.ToLower(strings.TrimSpace(hint.String()))
		}
	}

	img, _, err := decodeImageBytes(data)
	if err != nil {
		return nil, fmt.Errorf("barcode.decode: %w", err)
	}

	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return nil, fmt.Errorf("barcode.decode: bitmap: %w", err)
	}

	if formatHint != "" {
		reader := formatReader(formatHint)
		if reader == nil {
			return nil, fmt.Errorf("barcode.decode: unsupported format %q (decoder accepts: %s)", formatHint, strings.Join(barcodeDecodableFormats, ", "))
		}
		result, err := reader.DecodeWithoutHints(bitmap)
		if err != nil {
			return nil, fmt.Errorf("barcode.decode: %s reader: %w", formatHint, err)
		}
		out := gozxingFormatString(result.GetBarcodeFormat())
		if out == "" {
			out = formatHint
		}
		return map[string]any{"format": out, "text": result.GetText()}, nil
	}

	// Auto-detect: walk every reader in autoDecodeOrder; first hit
	// wins. We do NOT short-circuit on "format-specific" errors —
	// gozxing returns NotFoundException for every failing reader and
	// only the very last error is worth reporting if none of them
	// succeed (callers don't care which 11 readers failed).
	var lastErr error
	for _, name := range autoDecodeOrder {
		reader := formatReader(name)
		if reader == nil {
			continue
		}
		result, err := reader.DecodeWithoutHints(bitmap)
		if err != nil {
			lastErr = err
			continue
		}
		out := gozxingFormatString(result.GetBarcodeFormat())
		if out == "" {
			out = name
		}
		return map[string]any{"format": out, "text": result.GetText()}, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("barcode.decode: no barcode recognised (tried %d formats; last error: %w)", len(autoDecodeOrder), lastErr)
	}
	return nil, errors.New("barcode.decode: no barcode recognised")
}

// jsArgToBytes converts the first JS argument into a []byte. Accepts
// strings (UTF-8 bytes), ArrayBuffer, and Uint8Array. Anything else
// returns an error rather than silently fmt.Sprint-ing.
func jsArgToBytes(v goja.Value) ([]byte, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, errors.New("data is nil")
	}
	switch ex := v.Export().(type) {
	case string:
		return []byte(ex), nil
	case []byte:
		return ex, nil
	case goja.ArrayBuffer:
		return ex.Bytes(), nil
	}
	// Uint8Array exports as []byte through goja so the case above
	// catches it. Fall-through: hand back a typed error so the user
	// knows what kinds of JS-side payload work.
	return nil, fmt.Errorf("data must be a string, ArrayBuffer, or Uint8Array (got %T)", v.Export())
}

// jpegMagic / webpMagic check the magic-byte prefix for each format.
// Unused at the binding layer (we let image.Decode sniff), but kept
// around for unit tests that want to verify our magic-byte
// expectations without round-tripping through gozxing.
var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 'P', 'N', 'G'}
	webpRIFF  = []byte("RIFF")
)

func sniffImageFormat(b []byte) string {
	switch {
	case len(b) >= 4 && bytes.HasPrefix(b, pngMagic):
		return "png"
	case len(b) >= 3 && bytes.HasPrefix(b, jpegMagic):
		return "jpeg"
	case len(b) >= 12 && bytes.HasPrefix(b, webpRIFF) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "webp"
	default:
		return ""
	}
}

// unused-imports guard — image/png + image/jpeg are pulled in solely
// for their init-side decoder registration so image.Decode recognises
// them. Reference them here to keep the static-analysis tools quiet.
var _ = png.Decode
var _ = jpeg.Decode
