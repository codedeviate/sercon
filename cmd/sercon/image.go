package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"github.com/disintegration/imaging"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
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

// filterByName maps a filter name to an imaging.ResampleFilter (default Lanczos).
func filterByName(name string) imaging.ResampleFilter {
	switch strings.ToLower(name) {
	case "nearest":
		return imaging.NearestNeighbor
	case "linear":
		return imaging.Linear
	case "box":
		return imaging.Box
	case "catmullrom":
		return imaging.CatmullRom
	default:
		return imaging.Lanczos
	}
}

// imageNamespace builds the top-level `image` global.
func imageNamespace(vm *goja.Runtime, _ *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			data, err := os.ReadFile(path) //nolint:gosec // user-provided path is intentional
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("image.open: %w", err)))
			}
			img, format, err := decodeImage(data)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return newImageHandle(vm, img, format)
		},
		"decode": func(call goja.FunctionCall) goja.Value {
			data, ok := call.Argument(0).Export().([]byte)
			if !ok {
				panic(vm.NewTypeError("image.decode: expected a Uint8Array"))
			}
			img, format, err := decodeImage(data)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return newImageHandle(vm, img, format)
		},
		"rasterizeSVG": func(call goja.FunctionCall) goja.Value {
			var data []byte
			arg := call.Argument(0)
			if s, ok := arg.Export().(string); ok {
				b, err := os.ReadFile(s) //nolint:gosec // intentional
				if err != nil {
					panic(vm.NewGoError(fmt.Errorf("image.rasterizeSVG: %w", err)))
				}
				data = b
			} else if b, ok := arg.Export().([]byte); ok {
				data = b
			} else {
				panic(vm.NewTypeError("image.rasterizeSVG: expected a path string or Uint8Array"))
			}
			opts := call.Argument(1).ToObject(vm)
			w := int(opts.Get("width").ToInteger())
			h := int(opts.Get("height").ToInteger())
			if w <= 0 || h <= 0 {
				panic(vm.NewTypeError("image.rasterizeSVG: opts.width and opts.height (>0) are required"))
			}
			img, err := rasterizeSVG(data, w, h)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return newImageHandle(vm, img, "svg")
		},
	}
}

// rasterizeSVG renders an SVG subset to an RGBA image at w×h (oksvg/rasterx).
func rasterizeSVG(data []byte, w, h int) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: svg parse: %w", err)
	}
	icon.SetTarget(0, 0, float64(w), float64(h))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)
	return rgba, nil
}

// newImageHandle wraps a decoded image in a goja object with read-only
// width/height/format and chainable transform methods. Each transform applies
// an imaging op and returns a fresh handle (immutable chaining).
func newImageHandle(vm *goja.Runtime, img image.Image, srcFormat string) goja.Value {
	obj := vm.NewObject()
	b := img.Bounds()
	_ = obj.Set("width", b.Dx())
	_ = obj.Set("height", b.Dy())
	_ = obj.Set("format", srcFormat)

	wrap := func(out image.Image) goja.Value { return newImageHandle(vm, out, srcFormat) }
	argInt := func(call goja.FunctionCall, i int) int { return int(call.Argument(i).ToInteger()) }
	argFloat := func(call goja.FunctionCall, i int) float64 { return call.Argument(i).ToFloat() }

	_ = obj.Set("resize", func(call goja.FunctionCall) goja.Value {
		w, h := argInt(call, 0), argInt(call, 1)
		filter := imaging.Lanczos
		if o := call.Argument(2); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
			if fv := o.ToObject(vm).Get("filter"); fv != nil && !goja.IsUndefined(fv) {
				filter = filterByName(fv.String())
			}
		}
		return wrap(imaging.Resize(img, w, h, filter)) // w or h == 0 preserves aspect
	})
	_ = obj.Set("fit", func(call goja.FunctionCall) goja.Value {
		return wrap(imaging.Fit(img, argInt(call, 0), argInt(call, 1), imaging.Lanczos))
	})
	_ = obj.Set("thumbnail", func(call goja.FunctionCall) goja.Value {
		return wrap(imaging.Fill(img, argInt(call, 0), argInt(call, 1), imaging.Center, imaging.Lanczos))
	})
	_ = obj.Set("crop", func(call goja.FunctionCall) goja.Value {
		x, y, w, h := argInt(call, 0), argInt(call, 1), argInt(call, 2), argInt(call, 3)
		if w <= 0 || h <= 0 || x < img.Bounds().Min.X || y < img.Bounds().Min.Y ||
			x+w > img.Bounds().Max.X || y+h > img.Bounds().Max.Y {
			panic(vm.NewTypeError(fmt.Sprintf("image.crop: rect (%d,%d,%d,%d) out of bounds %v", x, y, w, h, img.Bounds())))
		}
		return wrap(imaging.Crop(img, image.Rect(x, y, x+w, y+h)))
	})
	_ = obj.Set("rotate", func(call goja.FunctionCall) goja.Value {
		return wrap(imaging.Rotate(img, argFloat(call, 0), color.NRGBA{0, 0, 0, 0}))
	})
	_ = obj.Set("rotate90", func(goja.FunctionCall) goja.Value { return wrap(imaging.Rotate90(img)) })
	_ = obj.Set("rotate180", func(goja.FunctionCall) goja.Value { return wrap(imaging.Rotate180(img)) })
	_ = obj.Set("rotate270", func(goja.FunctionCall) goja.Value { return wrap(imaging.Rotate270(img)) })
	_ = obj.Set("flipH", func(goja.FunctionCall) goja.Value { return wrap(imaging.FlipH(img)) })
	_ = obj.Set("flipV", func(goja.FunctionCall) goja.Value { return wrap(imaging.FlipV(img)) })
	_ = obj.Set("brightness", func(call goja.FunctionCall) goja.Value { return wrap(imaging.AdjustBrightness(img, argFloat(call, 0))) })
	_ = obj.Set("contrast", func(call goja.FunctionCall) goja.Value { return wrap(imaging.AdjustContrast(img, argFloat(call, 0))) })
	_ = obj.Set("gamma", func(call goja.FunctionCall) goja.Value { return wrap(imaging.AdjustGamma(img, argFloat(call, 0))) })
	_ = obj.Set("saturation", func(call goja.FunctionCall) goja.Value { return wrap(imaging.AdjustSaturation(img, argFloat(call, 0))) })
	_ = obj.Set("sharpen", func(call goja.FunctionCall) goja.Value { return wrap(imaging.Sharpen(img, argFloat(call, 0))) })
	_ = obj.Set("blur", func(call goja.FunctionCall) goja.Value { return wrap(imaging.Blur(img, argFloat(call, 0))) })
	_ = obj.Set("grayscale", func(goja.FunctionCall) goja.Value { return wrap(imaging.Grayscale(img)) })
	_ = obj.Set("invert", func(goja.FunctionCall) goja.Value { return wrap(imaging.Invert(img)) })

	otherImg := func(call goja.FunctionCall, i int) image.Image {
		o := call.Argument(i).ToObject(vm)
		gi := o.Get("__goimage")
		if gi == nil || goja.IsUndefined(gi) {
			panic(vm.NewTypeError("expected an Image handle"))
		}
		mi, ok := gi.Export().(image.Image)
		if !ok {
			panic(vm.NewTypeError("expected an Image handle"))
		}
		return mi
	}
	_ = obj.Set("__goimage", img) // internal: lets overlay/paste recover the Go image
	_ = obj.Set("overlay", func(call goja.FunctionCall) goja.Value {
		over := otherImg(call, 0)
		op := 1.0
		if len(call.Arguments) > 3 {
			op = argFloat(call, 3)
		}
		return wrap(imaging.Overlay(img, over, image.Pt(argInt(call, 1), argInt(call, 2)), op))
	})
	_ = obj.Set("paste", func(call goja.FunctionCall) goja.Value {
		return wrap(imaging.Paste(img, otherImg(call, 0), image.Pt(argInt(call, 1), argInt(call, 2))))
	})

	_ = obj.Set("bytes", func(call goja.FunctionCall) goja.Value {
		format := strings.ToLower(call.Argument(0).String())
		o := encodeOpts{}
		if a := call.Argument(1); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
			if qv := a.ToObject(vm).Get("quality"); qv != nil && !goja.IsUndefined(qv) {
				o.quality = int(qv.ToInteger())
			}
		}
		out, err := encodeImage(img, format, o)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(out) // []byte → Uint8Array
	})
	_ = obj.Set("save", func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()
		format := ""
		o := encodeOpts{}
		if a := call.Argument(1); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
			ao := a.ToObject(vm)
			if fv := ao.Get("format"); fv != nil && !goja.IsUndefined(fv) {
				format = fv.String()
			}
			if qv := ao.Get("quality"); qv != nil && !goja.IsUndefined(qv) {
				o.quality = int(qv.ToInteger())
			}
		}
		if format == "" {
			f, err := inferFormatFromPath(path)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			format = f
		}
		out, err := encodeImage(img, format, o)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		if err := os.WriteFile(path, out, 0o644); err != nil { //nolint:gosec // intentional
			panic(vm.NewGoError(fmt.Errorf("image.save: %w", err)))
		}
		return goja.Undefined()
	})

	return obj
}
