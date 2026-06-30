// cmd/sercon/codec_doc.go
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

// docModel is the uniform document model: the full extracted text plus a
// paragraph breakdown. Extraction is best-effort (extraction-grade).
type docModel struct {
	format     string
	text       string
	paragraphs []string
}

// splitParagraphs splits flat text into paragraphs on blank-line runs, trimming
// each block and dropping empties. Used by readers that yield only flat text
// (pdf, doc).
func splitParagraphs(text string) []string {
	var out []string
	for _, blk := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if b := strings.TrimSpace(blk); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// joinParagraphs renders paragraphs as text, blocks separated by a blank line.
func joinParagraphs(paras []string) string { return strings.Join(paras, "\n\n") }

// docFormat describes a codec.doc format's read/write capability. The
// docFormats table is the single source of truth for read dispatch, write
// rejection, and codec.doc.formats().
type docFormat struct{ read, write bool }

var docFormats = map[string]docFormat{
	"pdf":  {read: true, write: false},
	"docx": {read: true, write: true},
	"doc":  {read: true, write: false},
	"rtf":  {read: true, write: true},
	"odt":  {read: true, write: true},
}

// sniffDocFormat detects the document format from content; "" if unknown.
func sniffDocFormat(data []byte) string {
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return "pdf"
	}
	if bytes.HasPrefix(data, []byte(`{\rtf`)) {
		return "rtf"
	}
	if bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		return "doc"
	}
	if len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04 {
		if zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
			for _, f := range zr.File {
				if f.Name == "mimetype" {
					if rc, e := f.Open(); e == nil {
						mt, _ := io.ReadAll(io.LimitReader(rc, 256))
						_ = rc.Close()
						if strings.TrimSpace(string(mt)) == odtMimetype {
							return "odt"
						}
					}
				}
			}
			for _, f := range zr.File {
				if f.Name == "word/document.xml" {
					return "docx"
				}
			}
		}
	}
	return ""
}

// docSrcBytes reads a path string (returning its lowercased extension) or a
// Uint8Array (no extension).
func docSrcBytes(vm *goja.Runtime, arg goja.Value) (data []byte, ext string) {
	if s, ok := arg.Export().(string); ok {
		b, err := os.ReadFile(s) //nolint:gosec // user-provided path is intentional
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("codec.doc.read: %w", err)))
		}
		return b, strings.ToLower(filepath.Ext(s))
	}
	if b, ok := arg.Export().([]byte); ok {
		return b, ""
	}
	panic(vm.NewTypeError("codec.doc.read: expected a path string or Uint8Array"))
}

// docModelToJS surfaces a docModel as { format, text, paragraphs }.
func docModelToJS(vm *goja.Runtime, m docModel) goja.Value {
	paras := make([]any, len(m.paragraphs))
	for i, p := range m.paragraphs {
		paras[i] = p
	}
	return vm.ToValue(map[string]any{"format": m.format, "text": m.text, "paragraphs": paras})
}

// docModelArg coerces the write model ({paragraphs}|{text}|string) to paragraphs.
func docModelArg(vm *goja.Runtime, arg goja.Value) []string {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(vm.NewTypeError("codec.doc.write: model is required ({paragraphs}|{text}|string)"))
	}
	switch v := arg.Export().(type) {
	case string:
		return docSplitWriteText(v)
	case map[string]any:
		if pr, ok := v["paragraphs"]; ok {
			if arr, ok := pr.([]any); ok {
				out := make([]string, len(arr))
				for i, e := range arr {
					if e == nil { // JS null/undefined → empty paragraph, not "<nil>"
						continue
					}
					out[i] = fmt.Sprintf("%v", e)
				}
				return out
			}
		}
		if tx, ok := v["text"]; ok {
			if s, ok := tx.(string); ok {
				return docSplitWriteText(s)
			}
		}
		panic(vm.NewTypeError("codec.doc.write: model must have paragraphs[] or text"))
	default:
		panic(vm.NewTypeError("codec.doc.write: model must be an object or string"))
	}
}

// docSplitWriteText splits write input into one paragraph per line, dropping
// trailing blank lines.
func docSplitWriteText(s string) []string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// docNamespace returns the codec.doc sub-namespace.
func docNamespace(vm *goja.Runtime) map[string]any {
	throwErr := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"read": func(call goja.FunctionCall) goja.Value {
			data, ext := docSrcBytes(vm, call.Argument(0))
			format := ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if fv := o.ToObject(vm).Get("format"); fv != nil && !goja.IsUndefined(fv) {
					format = strings.ToLower(fv.String())
				}
			}
			if format == "" {
				switch ext {
				case ".pdf":
					format = "pdf"
				case ".docx":
					format = "docx"
				case ".doc":
					format = "doc"
				case ".rtf":
					format = "rtf"
				case ".odt":
					format = "odt"
				default:
					format = sniffDocFormat(data)
				}
			}
			var m docModel
			var err error
			switch format {
			case "pdf":
				m, err = readPDF(data)
			case "docx":
				m, err = readDOCX(data)
			case "doc":
				m, err = readDOC(data)
			case "rtf":
				m, err = readRTF(data)
			case "odt":
				m, err = readODT(data)
			default:
				return throwErr(fmt.Errorf("codec.doc.read: unrecognized document format (pdf, docx, doc, rtf, odt)"))
			}
			if err != nil {
				return throwErr(err)
			}
			return docModelToJS(vm, m)
		},
		"write": func(call goja.FunctionCall) goja.Value {
			paras := docModelArg(vm, call.Argument(0))
			format, dest := "", ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				obj := o.ToObject(vm)
				if fv := obj.Get("format"); fv != nil && !goja.IsUndefined(fv) {
					format = strings.ToLower(fv.String())
				}
				if dv := obj.Get("dest"); dv != nil && !goja.IsUndefined(dv) {
					dest = dv.String()
				}
			}
			if format == "" && dest != "" {
				switch strings.ToLower(filepath.Ext(dest)) {
				case ".docx":
					format = "docx"
				case ".rtf":
					format = "rtf"
				case ".odt":
					format = "odt"
				case ".pdf":
					format = "pdf"
				case ".doc":
					format = "doc"
				}
			}
			if f, ok := docFormats[format]; ok && !f.write {
				return throwErr(fmt.Errorf("codec.doc.write: %s is read-only (extract/convert only); write docx, rtf, or odt", format))
			}
			var out []byte
			var err error
			switch format {
			case "docx":
				out, err = writeDOCX(paras)
			case "rtf":
				out, err = writeRTF(paras)
			case "odt":
				out, err = writeODT(paras)
			default:
				return throwErr(fmt.Errorf("codec.doc.write: format is required (docx, rtf, odt)"))
			}
			if err != nil {
				return throwErr(err)
			}
			if dest != "" {
				if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
					return throwErr(fmt.Errorf("codec.doc.write: %w", werr))
				}
				return vm.ToValue(map[string]any{"path": dest})
			}
			return vm.ToValue(map[string]any{"bytes": out})
		},
		"formats": func(call goja.FunctionCall) goja.Value {
			out := map[string]any{}
			for name, f := range docFormats {
				out[name] = map[string]any{"read": f.read, "write": f.write}
			}
			return vm.ToValue(out)
		},
	}
}
