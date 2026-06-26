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

// optAutoOrient reports whether opts is an object with autoOrient === true.
func optAutoOrient(vm *goja.Runtime, arg goja.Value) bool {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return false
	}
	o, ok := arg.Export().(map[string]any)
	if !ok {
		return false
	}
	b, _ := o["autoOrient"].(bool)
	return b
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
			if optAutoOrient(vm, call.Argument(1)) {
				img = applyOrientation(img, exifOrientation(data, format))
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
			if optAutoOrient(vm, call.Argument(1)) {
				img = applyOrientation(img, exifOrientation(data, format))
			}
			return newImageHandle(vm, img, format)
		},
		"decodeFrames": func(call goja.FunctionCall) goja.Value {
			data := imageSrcBytes(vm, call.Argument(0), "decodeFrames")
			doc, err := decodeFramesAny(data)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return framesToJS(vm, doc)
		},
		"encodeFrames": func(call goja.FunctionCall) goja.Value {
			doc := jsToAnimDoc(vm, call.Argument(0))
			format := "gif"
			dest := ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				obj := o.ToObject(vm)
				if f := obj.Get("format"); f != nil && !goja.IsUndefined(f) {
					format = strings.ToLower(f.String())
				}
				if d := obj.Get("dest"); d != nil && !goja.IsUndefined(d) {
					dest = d.String()
				}
			}
			var out []byte
			var err error
			switch format {
			case "gif":
				out, err = encodeFramesGIF(doc)
			case "apng":
				out, err = encodeFramesAPNG(doc)
			default:
				panic(vm.NewGoError(fmt.Errorf("image.encodeFrames: unsupported format %q (gif, apng)", format)))
			}
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if dest != "" {
				if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
					panic(vm.NewGoError(fmt.Errorf("image.encodeFrames: %w", werr)))
				}
				return vm.ToValue(map[string]any{"format": format, "path": dest})
			}
			return vm.ToValue(map[string]any{"format": format, "bytes": out})
		},
		"exif": exifNamespace(vm),
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

// imageSrcBytes reads a path string or Uint8Array into bytes (shared by the
// frame ops); mirrors the open/decode arg handling.
func imageSrcBytes(vm *goja.Runtime, arg goja.Value, op string) []byte {
	if s, ok := arg.Export().(string); ok {
		b, err := os.ReadFile(s) //nolint:gosec // user-provided path is intentional
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("image.%s: %w", op, err)))
		}
		return b
	}
	if b, ok := arg.Export().([]byte); ok {
		return b
	}
	panic(vm.NewTypeError("image.%s: expected a path string or Uint8Array", op))
}

// framesToJS renders an animDoc as the script-facing { format, width, height,
// loopCount, frames:[{image, delayMs, xOffset, yOffset, disposal, blend}] }.
func framesToJS(vm *goja.Runtime, doc animDoc) goja.Value {
	frames := make([]any, len(doc.frames))
	for i, f := range doc.frames {
		fr := map[string]any{
			"image":    newImageHandle(vm, f.img, doc.format),
			"delayMs":  f.delayMs,
			"xOffset":  f.xOffset,
			"yOffset":  f.yOffset,
			"disposal": f.disposal,
		}
		if doc.format == "apng" {
			fr["blend"] = f.blend
		}
		frames[i] = fr
	}
	return vm.ToValue(map[string]any{
		"format": doc.format, "width": doc.width, "height": doc.height,
		"loopCount": doc.loopCount, "frames": frames,
	})
}

// jsToAnimDoc parses the JS spec object into an animDoc, recovering each
// frame's Go image via the handle's __goimage field (as overlay/paste do).
func jsToAnimDoc(vm *goja.Runtime, specArg goja.Value) animDoc {
	if specArg == nil || goja.IsUndefined(specArg) || goja.IsNull(specArg) {
		panic(vm.NewTypeError("image.encodeFrames: spec object with a frames array is required"))
	}
	spec := specArg.ToObject(vm)
	jsInt := func(o *goja.Object, key string) int {
		v := o.Get(key)
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			return 0
		}
		return int(v.ToInteger())
	}
	doc := animDoc{
		width:     jsInt(spec, "width"),
		height:    jsInt(spec, "height"),
		loopCount: jsInt(spec, "loopCount"),
	}
	framesV := spec.Get("frames")
	if framesV == nil || goja.IsUndefined(framesV) {
		panic(vm.NewTypeError("image.encodeFrames: spec.frames is required"))
	}
	arr := framesV.ToObject(vm)
	n := int(arr.Get("length").ToInteger())
	for i := 0; i < n; i++ {
		fo := arr.Get(fmt.Sprintf("%d", i)).ToObject(vm)
		gi := fo.Get("image")
		if gi == nil || goja.IsUndefined(gi) {
			panic(vm.NewTypeError("image.encodeFrames: each frame needs an Image handle"))
		}
		giObj := gi.ToObject(vm).Get("__goimage")
		if giObj == nil || goja.IsUndefined(giObj) {
			panic(vm.NewTypeError("image.encodeFrames: frame.image must be an Image handle"))
		}
		img, ok := giObj.Export().(image.Image)
		if !ok {
			panic(vm.NewTypeError("image.encodeFrames: frame.image must be an Image handle"))
		}
		f := animFrame{img: img,
			delayMs: jsInt(fo, "delayMs"), xOffset: jsInt(fo, "xOffset"), yOffset: jsInt(fo, "yOffset"),
			disposal: "none", blend: "over"}
		if d := fo.Get("disposal"); d != nil && !goja.IsUndefined(d) && !goja.IsNull(d) {
			f.disposal = d.String()
		}
		if bl := fo.Get("blend"); bl != nil && !goja.IsUndefined(bl) && !goja.IsNull(bl) {
			f.blend = bl.String()
		}
		doc.frames = append(doc.frames, f)
	}
	return doc
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
	_ = obj.Set("orient", func(call goja.FunctionCall) goja.Value {
		nf := call.Argument(0).ToFloat()
		n := int(nf)
		if float64(n) != nf || n < 1 || n > 8 {
			panic(vm.NewTypeError("image.orient: n must be an integer 1..8 (EXIF orientation)"))
		}
		return wrap(applyOrientation(img, n))
	})
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
