package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/aztec"
	"github.com/boombuler/barcode/codabar"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/pdf417"
	"github.com/boombuler/barcode/qr"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// barcodeFormats enumerates the symbology names accepted by codec.barcode.encode.
// All ten map onto the boombuler/barcode toolkit's pure-Go encoders. Names
// are lowercase + alphanumeric so JS callers don't have to remember
// punctuation.
var barcodeFormats = []string{
	"qr", "datamatrix", "aztec", "pdf417",
	"code128", "code39", "codabar",
	"ean13", "ean8", "upca",
}

// barcodeNamespace builds the `codec.barcode.*` member map. `encode` returns
// a PNG payload as an ArrayBuffer (scripts wrap with `new Uint8Array(...)`
// to inspect bytes; a typical use is to base64-encode for embedding).
func barcodeNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	formats := make([]string, len(barcodeFormats))
	copy(formats, barcodeFormats)
	decFormats := make([]string, len(barcodeDecodableFormats))
	copy(decFormats, barcodeDecodableFormats)
	return map[string]any{
		"encode":           scriptengine.PromisifyAsync(vm, loop, barcodeEncodeCall),
		"decode":           scriptengine.PromisifyAsync(vm, loop, barcodeDecodeCall),
		"formats":          func() []string { return formats },
		"decodableFormats": func() []string { return decFormats },
	}
}

// barcodeEncodeCall reads (format, data, opts?) from a JS call, dispatches
// to the right boombuler encoder, scales to the requested pixel dimensions
// (with format-appropriate defaults), and PNG-encodes the result.
func barcodeEncodeCall(_ context.Context, call goja.FunctionCall) ([]byte, error) {
	format := strings.ToLower(call.Argument(0).String())
	data := call.Argument(1).String()
	// barcodeEncodeCall takes 3 positional args (format, data, opts), so the
	// 2-arg optsAsMap helper would mistake `data` for opts. Pull the third
	// arg out by hand. Same shape as the diff.compare / archive.extract fixes.
	var opts map[string]any
	if len(call.Arguments) >= 3 {
		arg := call.Argument(2)
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			if m, ok := arg.Export().(map[string]any); ok {
				opts = m
			}
		}
	}
	width := optInt(opts, "width", 0)
	height := optInt(opts, "height", 0)

	bc, err := buildBarcode(format, data)
	if err != nil {
		return nil, err
	}

	// Pick a sensible default size when the caller didn't ask for one.
	// 2D codes default to square 256; 1D barcodes get a wider rectangle
	// because the bars are themselves narrow.
	if width == 0 || height == 0 {
		switch format {
		case "qr", "datamatrix", "aztec":
			if width == 0 {
				width = 256
			}
			if height == 0 {
				height = 256
			}
		default:
			if width == 0 {
				width = 400
			}
			if height == 0 {
				height = 120
			}
		}
	}

	scaled, err := barcode.Scale(bc, width, height)
	if err != nil {
		return nil, fmt.Errorf("scale barcode: %w", err)
	}

	// opts.quietZone pads the rendered bars with a white margin. EAN /
	// UPC (and many real-world scanners, gozxing included) REQUIRE this
	// margin per ISO/IEC 15420 — boombuler emits bars edge-to-edge, so
	// without padding a sercon-encoded EAN won't decode. Accept either
	// `true` (a sensible default margin) or an explicit pixel count.
	final := withQuietZone(scaled, quietZonePixels(opts, width))
	var buf bytes.Buffer
	if err := png.Encode(&buf, final); err != nil {
		return nil, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), nil
}

// quietZonePixels resolves opts.quietZone into a pixel margin.
// Absent / false / 0 → no padding. `true` → 10% of the barcode
// width (a generous default that satisfies the EAN/UPC spec's
// minimum). A number → that many pixels on each side. Negative
// values are clamped to zero.
func quietZonePixels(opts map[string]any, width int) int {
	if opts == nil {
		return 0
	}
	v, ok := opts["quietZone"]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case bool:
		if !t {
			return 0
		}
		px := width / 10
		if px < 10 {
			px = 10 // floor so tiny barcodes still get a usable margin
		}
		return px
	case int64:
		return clampZero(int(t))
	case int:
		return clampZero(t)
	case float64:
		return clampZero(int(t))
	default:
		return 0
	}
}

func clampZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// withQuietZone pastes `src` onto a white canvas with `pad` pixels of
// margin on every side. A zero margin returns src unchanged (no
// allocation). The margin is symmetric — top/bottom get the same pad
// as left/right, which is more than the spec strictly requires
// vertically but keeps the output visually centred.
func withQuietZone(src image.Image, pad int) image.Image {
	if pad <= 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	canvas := image.NewNRGBA(image.Rect(0, 0, w+2*pad, h+2*pad))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(pad, pad, pad+w, pad+h), src, b.Min, draw.Src)
	return canvas
}

// buildBarcode dispatches by format name to the matching boombuler encoder.
// Each encoder has its own knobs (QR error-correction level, Code 39's
// checksum + extended-ASCII modes, …); we pick conservative defaults that
// match what most callers expect.
func buildBarcode(format, data string) (barcode.Barcode, error) {
	switch format {
	case "qr":
		// Medium error correction is the QR-spec default; auto mode picks
		// the right encoding (numeric / alphanumeric / byte) per content.
		return qr.Encode(data, qr.M, qr.Auto)
	case "datamatrix":
		return datamatrix.Encode(data)
	case "aztec":
		// Default to boombuler's recommended ECC% (33) and let the encoder
		// choose its own layer count.
		return aztec.Encode([]byte(data), 33, 0)
	case "pdf417":
		// Security level 5 of 8 — middle of the road.
		return pdf417.Encode(data, 5)
	case "code128":
		return code128.Encode(data)
	case "code39":
		// Include a Mod-43 checksum (the most-asked-for variant) and
		// disable full-ASCII to match how most readers are configured.
		return code39.Encode(data, true, false)
	case "codabar":
		return codabar.Encode(data)
	case "ean13", "ean8", "upca":
		// boombuler/ean dispatches on the content length, so all three
		// EAN/UPC variants share an encoder. We still accept distinct
		// format names so the caller can document intent.
		return ean.Encode(data)
	default:
		return nil, errors.New("unknown barcode format: " + format +
			" (supported: " + strings.Join(barcodeFormats, ", ") + ")")
	}
}

// optInt pulls a numeric option from a JS opts object. Accepts int64 /
// int / float64 (int64 is what goja exports for JS integers; int shows
// up when Go-side test harnesses build the map directly; float64 covers
// non-integer JS literals). Falls back for everything else.
func optInt(opts map[string]any, key string, fallback int) int {
	if opts == nil {
		return fallback
	}
	v, ok := opts[key]
	if !ok {
		return fallback
	}
	switch t := v.(type) {
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	}
	return fallback
}
