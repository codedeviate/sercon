// cmd/sercon/exif.go
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dop251/goja"
)

func exifNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"read":    func(call goja.FunctionCall) goja.Value { return exifRead(vm, call) },
		"write":   func(call goja.FunctionCall) goja.Value { return exifWrite(vm, call, modeMerge) },
		"replace": func(call goja.FunctionCall) goja.Value { return exifWrite(vm, call, modeReplace) },
		"clear":   func(call goja.FunctionCall) goja.Value { return exifClear(vm, call) },
	}
}

// readImageSrc resolves a goja argument to raw image bytes.
// arg may be a path string or a Uint8Array ([]byte).
func readImageSrc(vm *goja.Runtime, arg goja.Value, op string) []byte {
	if s, ok := arg.Export().(string); ok {
		b, err := os.ReadFile(s) //nolint:gosec // user-provided path is intentional
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("image.exif.%s: %w", op, err)))
		}
		return b
	}
	if b, ok := arg.Export().([]byte); ok {
		return b
	}
	panic(vm.NewTypeError("image.exif.%s: expected a path string or Uint8Array", op))
}

// sniffFormat returns the decoded format name via the existing decoder sniff.
// Returns "" when the format cannot be determined.
func sniffFormat(data []byte) string {
	_, format, err := decodeImage(data)
	if err != nil {
		return ""
	}
	return format
}

func exifRead(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	data := readImageSrc(vm, call.Argument(0), "read")
	format := sniffFormat(data)
	var doc exifDoc
	var err error
	switch format {
	case "jpeg", "png", "tiff":
		raw, e := extractRawExif(data, format)
		if errors.Is(e, errNoExif) {
			return vm.ToValue(map[string]any{}) // no EXIF → {}
		}
		if e != nil {
			panic(vm.NewGoError(fmt.Errorf("image.exif.read: %w", e)))
		}
		doc, err = readExifDsoprea(raw)
	case "heic", "avif", "cr2", "cr3", "nef", "arw", "dng", "raw":
		doc, err = readExifImagemeta(data)
	default:
		// imagemeta can sniff some formats decodeImage can't; try it as a last resort.
		doc, err = readExifImagemeta(data)
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("image.exif.read: %w", err)))
	}
	// Convert exifDoc (map[string]map[string]any) to map[string]any for goja.
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		out[k] = v
	}
	return vm.ToValue(out)
}

func exifWrite(vm *goja.Runtime, call goja.FunctionCall, mode writeMode) goja.Value {
	op := "write"
	if mode == modeReplace {
		op = "replace"
	}
	data := readImageSrc(vm, call.Argument(0), op)
	doc := gojaToExifDoc(vm, call.Argument(1), op)
	return exifApply(vm, data, doc, mode, op, call.Argument(2))
}

func exifClear(vm *goja.Runtime, call goja.FunctionCall) goja.Value {
	data := readImageSrc(vm, call.Argument(0), "clear")
	return exifApply(vm, data, nil, modeClear, "clear", call.Argument(1))
}

// exifApply runs the write/replace/clear for the sniffed format and returns
// { format, bytes } or { format, path } per opts.dest.
func exifApply(vm *goja.Runtime, data []byte, doc exifDoc, mode writeMode, op string, optsArg goja.Value) goja.Value {
	format := sniffFormat(data)
	var out []byte
	var err error
	switch format {
	case "jpeg":
		out, err = writeExifJPEG(data, doc, mode)
	case "png":
		out, err = writeExifPNG(data, doc, mode)
	default:
		panic(vm.NewGoError(fmt.Errorf("image.exif.%s: writing EXIF to %q is unsupported (read-only); supported: jpeg, png", op, fmtOrUnknown(format))))
	}
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("image.exif.%s: %w", op, err)))
	}

	dest := ""
	if optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
		if o := optsArg.ToObject(vm); o != nil {
			if d := o.Get("dest"); d != nil && !goja.IsUndefined(d) && !goja.IsNull(d) {
				dest = d.String()
			}
		}
	}
	if dest != "" {
		if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
			panic(vm.NewGoError(fmt.Errorf("image.exif.%s: %w", op, werr)))
		}
		return vm.ToValue(map[string]any{"format": format, "path": dest})
	}
	return vm.ToValue(map[string]any{"format": format, "bytes": out})
}

func fmtOrUnknown(f string) string {
	if f == "" {
		return "unknown"
	}
	return f
}

// gojaToExifDoc converts the JS data object into an exifDoc (group → tag → value).
func gojaToExifDoc(vm *goja.Runtime, arg goja.Value, op string) exifDoc {
	m, ok := arg.Export().(map[string]any)
	if !ok {
		panic(vm.NewTypeError("image.exif.%s: data must be an object grouped by image/exif/gps/thumbnail", op))
	}
	doc := exifDoc{}
	for group, gv := range m {
		tags, ok := gv.(map[string]any)
		if !ok {
			panic(vm.NewTypeError("image.exif.%s: group %q must be an object", op, group))
		}
		doc[group] = tags
	}
	return doc
}
