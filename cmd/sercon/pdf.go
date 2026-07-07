// cmd/sercon/pdf.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// validatePDFFormat lowercases f, accepts png|jpeg|tiff (default png), and
// returns the pdftoppm flag for it ("-png" / "-jpeg" / "-tiff").
func validatePDFFormat(f string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "", "png":
		return "-png", nil
	case "jpeg", "jpg":
		return "-jpeg", nil
	case "tiff", "tif":
		return "-tiff", nil
	default:
		return "", fmt.Errorf("invalid format %q (png|jpeg|tiff)", f)
	}
}

// parsePDFPages parses "", "N", or "F-L" into 1-based first/last bounds.
// "" → (0,0) meaning "all pages". Non-positive or reversed ranges error.
func parsePDFPages(spec string) (first, last int, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, nil
	}
	if !strings.Contains(spec, "-") {
		n, e := strconv.Atoi(spec)
		if e != nil || n < 1 {
			return 0, 0, fmt.Errorf("invalid page %q (expect a positive integer)", spec)
		}
		return n, n, nil
	}
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid page range %q (expect F-L)", spec)
	}
	f, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	l, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if e1 != nil || e2 != nil || f < 1 || l < 1 || l < f {
		return 0, 0, fmt.Errorf("invalid page range %q (expect F-L with 1<=F<=L)", spec)
	}
	return f, l, nil
}

type pdfImageSpec struct {
	src, prefix, format string // format is the pdftoppm flag, e.g. "-png"
	firstPage, lastPage int
	dpi                 int
}

// buildPdfImageArgs builds the `pdftoppm` argv (deterministic order).
func buildPdfImageArgs(s pdfImageSpec) []string {
	flags := []string{s.format}
	if s.firstPage > 0 {
		flags = append(flags, "-f", strconv.Itoa(s.firstPage))
	}
	if s.lastPage > 0 {
		flags = append(flags, "-l", strconv.Itoa(s.lastPage))
	}
	if s.dpi > 0 {
		flags = append(flags, "-r", strconv.Itoa(s.dpi))
	}
	return safePathArgs(flags, s.src, s.prefix)
}

type pdfTextSpec struct {
	src, dest           string
	firstPage, lastPage int
	layout              bool
}

// buildPdfTextArgs builds the `pdftotext` argv; empty dest → stdout ("-").
func buildPdfTextArgs(s pdfTextSpec) []string {
	flags := []string{}
	if s.firstPage > 0 {
		flags = append(flags, "-f", strconv.Itoa(s.firstPage))
	}
	if s.lastPage > 0 {
		flags = append(flags, "-l", strconv.Itoa(s.lastPage))
	}
	if s.layout {
		flags = append(flags, "-layout")
	}
	dest := s.dest
	if dest == "" {
		dest = "-"
	}
	return safePathArgs(flags, s.src, dest)
}

type pdfHTMLSpec struct {
	src, dest           string
	firstPage, lastPage int
}

// buildPdfHTMLArgs builds the `pdftohtml` argv. -i ignores images (keeps the
// output a single self-contained file); -noframes emits one HTML document.
func buildPdfHTMLArgs(s pdfHTMLSpec) []string {
	flags := []string{"-i", "-noframes"}
	if s.firstPage > 0 {
		flags = append(flags, "-f", strconv.Itoa(s.firstPage))
	}
	if s.lastPage > 0 {
		flags = append(flags, "-l", strconv.Itoa(s.lastPage))
	}
	return safePathArgs(flags, s.src, s.dest)
}

// parsePdfInfo turns `pdfinfo` "Key: Value" output into the info() object.
func parsePdfInfo(out string) map[string]any {
	keyMap := map[string]string{
		"Title": "title", "Author": "author", "Creator": "creator",
		"Producer": "producer", "Page size": "pageSize", "File size": "fileSize",
		"PDF version": "pdfVersion", "CreationDate": "creationDate", "ModDate": "modDate",
	}
	info := map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "Pages":
			if n, err := strconv.Atoi(val); err == nil {
				info["pages"] = n
			}
		case "Encrypted":
			info["encrypted"] = strings.HasPrefix(strings.ToLower(val), "yes")
		case "Tagged":
			info["tagged"] = strings.EqualFold(val, "yes")
		default:
			if mapped, ok := keyMap[key]; ok && val != "" {
				info[mapped] = val
			}
		}
	}
	return info
}

const pdfTimeout = 60 * time.Second
const popplerInstallHint = "install poppler-utils: brew install poppler / apt install poppler-utils"

// optPages reads the pages option, accepting a string ("1-3") or a JS number
// (coerced to its integer string). Returns "" when absent.
func optPages(opts map[string]any) string {
	switch v := opts["pages"].(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	default:
		return ""
	}
}

// requirePDFSrc extracts the positional src path and optional opts map:
// every pdf op is called as op(src, opts?).
func requirePDFSrc(call goja.FunctionCall) (string, map[string]any, error) {
	src, ok := call.Argument(0).Export().(string)
	if !ok || strings.TrimSpace(src) == "" {
		return "", nil, fmt.Errorf("first argument must be a PDF path (string)")
	}
	opts := map[string]any{}
	if m, ok := call.Argument(1).Export().(map[string]any); ok {
		opts = m
	}
	return src, opts, nil
}

// pdfSrcArgs is the on-loop-extracted input shared by the op(src, opts?)
// pdf bindings.
type pdfSrcArgs struct {
	src  string
	opts map[string]any
}

// pdfSrcExtract returns the extract half for an op(src, opts?) pdf binding,
// wrapping argument errors with the binding name (e.g. "services.pdf.info").
func pdfSrcExtract(name string) func(goja.FunctionCall) (pdfSrcArgs, error) {
	return func(call goja.FunctionCall) (pdfSrcArgs, error) {
		src, opts, err := requirePDFSrc(call)
		if err != nil {
			return pdfSrcArgs{}, fmt.Errorf("%s: %w", name, err)
		}
		return pdfSrcArgs{src: src, opts: opts}, nil
	}
}

// pdfNoArgsExtract is the extract half for the argument-less pdf ops.
func pdfNoArgsExtract(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil }

func pdfVersionOp(ctx context.Context, _ struct{}) (any, error) {
	// poppler prints -v to stderr and exits 0; capture combined output so the
	// version line is read regardless of which stream it lands on.
	out, err := runTool(ctx, toolSpec{bin: "pdftoppm", argv: []string{"-v"}, timeout: 15 * time.Second, combinedOutput: true, installHint: popplerInstallHint})
	if err != nil {
		out, err = runTool(ctx, toolSpec{bin: "pdfinfo", argv: []string{"-v"}, timeout: 15 * time.Second, combinedOutput: true, installHint: popplerInstallHint})
		if err != nil {
			return nil, fmt.Errorf("services.pdf.version: %w", err)
		}
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return line, nil
}

func pdfInfoOp(ctx context.Context, args pdfSrcArgs) (any, error) {
	out, err := runTool(ctx, toolSpec{bin: "pdfinfo", argv: safePathArgs(nil, args.src), timeout: pdfTimeout, installHint: popplerInstallHint})
	if err != nil {
		return nil, fmt.Errorf("services.pdf.info: %w", err)
	}
	return parsePdfInfo(string(out)), nil
}

func pdfToImageOp(ctx context.Context, args pdfSrcArgs) (any, error) {
	src, opts := args.src, args.opts
	formatFlag, err := validatePDFFormat(optString(opts, "format", ""))
	if err != nil {
		return nil, fmt.Errorf("services.pdf.toImage: %w", err)
	}
	ext := strings.TrimPrefix(formatFlag, "-") // png|jpeg|tiff
	// Page selection: page (single) OR firstPage/lastPage.
	first, last := optInt(opts, "firstPage", 0), optInt(opts, "lastPage", 0)
	if p := optInt(opts, "page", 0); p > 0 {
		first, last = p, p
	}
	if first < 0 || last < 0 || (last > 0 && first > 0 && last < first) {
		return nil, fmt.Errorf("services.pdf.toImage: invalid page range")
	}
	dest := optString(opts, "dest", "")
	dpi := optInt(opts, "dpi", 0)

	// Single page, no dest → render to a temp prefix and read the one file back.
	returnBytes := dest == "" && first == last && first > 0
	// A multi-page or whole-document render writes multiple files; without a
	// dest they'd land in a temp dir we delete on return, handing the caller
	// dead paths. Require an explicit dest for that case.
	if dest == "" && !returnBytes {
		return nil, fmt.Errorf("services.pdf.toImage: multi-page or whole-document render requires a `dest` path; pass a single `page` to get bytes back")
	}
	prefix := dest
	var tmpDir string
	if dest == "" {
		d, derr := os.MkdirTemp("", "sercon-pdf-*")
		if derr != nil {
			return nil, fmt.Errorf("services.pdf.toImage: %w", derr)
		}
		tmpDir = d
		defer func() { _ = os.RemoveAll(tmpDir) }()
		prefix = filepath.Join(tmpDir, "page")
	}
	spec := pdfImageSpec{src: src, prefix: prefix, format: formatFlag, firstPage: first, lastPage: last, dpi: dpi}
	if _, rerr := runTool(ctx, toolSpec{bin: "pdftoppm", argv: buildPdfImageArgs(spec), timeout: pdfTimeout, installHint: popplerInstallHint}); rerr != nil {
		return nil, fmt.Errorf("services.pdf.toImage: %w", rerr)
	}
	written, gerr := globGenerated(prefix, ext)
	if gerr != nil {
		return nil, fmt.Errorf("services.pdf.toImage: %w", gerr)
	}
	if returnBytes {
		if len(written) == 0 {
			return nil, fmt.Errorf("services.pdf.toImage: no output produced for page %d", first)
		}
		data, rerr := os.ReadFile(written[0])
		if rerr != nil {
			return nil, fmt.Errorf("services.pdf.toImage: %w", rerr)
		}
		return scriptengine.NewOrdered().Set("format", ext).Set("page", first).Set("bytes", data), nil
	}
	paths := make([]any, len(written))
	for i, p := range written {
		paths[i] = p
	}
	return scriptengine.NewOrdered().Set("format", ext).Set("paths", paths), nil
}

// globGenerated returns the files pdftoppm produced for a prefix, sorted. It
// matches the literal singleton "<prefix>.<ext>" and the multi-page
// "<prefix>-N.<ext>" (N a run of digits — pdftoppm's page-number suffix).
//
// prefix is caller/user-controlled (it's derived from the `dest` option), so
// this deliberately avoids filepath.Glob: glob metacharacters (*, ?, [) in
// prefix would otherwise be interpreted as pattern syntax instead of literal
// text, corrupting the match (or matching unrelated files). Listing the
// directory and comparing the entry name against the literal prefix by string
// operations sidesteps that entirely — no part of prefix is ever compiled as
// a pattern.
func globGenerated(prefix, ext string) ([]string, error) {
	dir := filepath.Dir(prefix)
	base := filepath.Base(prefix)
	suffix := "." + ext
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, base) || !strings.HasSuffix(name, suffix) {
			continue
		}
		mid := name[len(base) : len(name)-len(suffix)]
		switch {
		case mid == "":
			// "<prefix>.<ext>" singleton.
		case mid[0] == '-' && isDigits(mid[1:]):
			// "<prefix>-N.<ext>".
		default:
			continue
		}
		matches = append(matches, filepath.Join(dir, name))
	}
	sort.Strings(matches)
	return matches, nil
}

// isDigits reports whether s is non-empty and consists entirely of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func pdfToTextOp(ctx context.Context, args pdfSrcArgs) (any, error) {
	src, opts := args.src, args.opts
	first, last, perr := parsePDFPages(optPages(opts))
	if perr != nil {
		return nil, fmt.Errorf("services.pdf.toText: %w", perr)
	}
	dest := optString(opts, "dest", "")
	spec := pdfTextSpec{src: src, dest: dest, firstPage: first, lastPage: last, layout: optBool(opts, "layout", false)}
	out, rerr := runTool(ctx, toolSpec{bin: "pdftotext", argv: buildPdfTextArgs(spec), timeout: pdfTimeout, installHint: popplerInstallHint, capHint: "pass a dest path to write large output to a file"})
	if rerr != nil {
		return nil, fmt.Errorf("services.pdf.toText: %w", rerr)
	}
	if dest != "" {
		return scriptengine.NewOrdered().Set("path", dest), nil
	}
	return string(out), nil
}

func pdfToHTMLOp(ctx context.Context, args pdfSrcArgs) (any, error) {
	src, opts := args.src, args.opts
	first, last, perr := parsePDFPages(optPages(opts))
	if perr != nil {
		return nil, fmt.Errorf("services.pdf.toHtml: %w", perr)
	}
	dest := optString(opts, "dest", "")
	target := dest
	var tmpDir string
	if dest == "" {
		d, derr := os.MkdirTemp("", "sercon-pdf-*")
		if derr != nil {
			return nil, fmt.Errorf("services.pdf.toHtml: %w", derr)
		}
		tmpDir = d
		defer func() { _ = os.RemoveAll(tmpDir) }()
		target = filepath.Join(tmpDir, "out.html")
	}
	spec := pdfHTMLSpec{src: src, dest: target, firstPage: first, lastPage: last}
	if _, rerr := runTool(ctx, toolSpec{bin: "pdftohtml", argv: buildPdfHTMLArgs(spec), timeout: pdfTimeout, installHint: popplerInstallHint}); rerr != nil {
		return nil, fmt.Errorf("services.pdf.toHtml: %w", rerr)
	}
	if dest != "" {
		return scriptengine.NewOrdered().Set("path", dest), nil
	}
	data, rerr := os.ReadFile(target)
	if rerr != nil {
		return nil, fmt.Errorf("services.pdf.toHtml: read output: %w", rerr)
	}
	return string(data), nil
}

// pdfNamespace builds the services.pdf member map.
func pdfNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	backend := any(nil)
	if toolAvailable("pdftoppm") {
		backend = "poppler"
	}
	return map[string]any{
		"available": toolAvailable("pdftoppm"),
		"backend":   backend,
		"tools": map[string]any{
			"pdftoppm":  toolAvailable("pdftoppm"),
			"pdftotext": toolAvailable("pdftotext"),
			"pdftohtml": toolAvailable("pdftohtml"),
			"pdfinfo":   toolAvailable("pdfinfo"),
		},
		"version": scriptengine.PromisifyAsync(vm, loop, pdfNoArgsExtract, pdfVersionOp),
		"info":    scriptengine.PromisifyAsync(vm, loop, pdfSrcExtract("services.pdf.info"), pdfInfoOp),
		"toImage": scriptengine.PromisifyAsync(vm, loop, pdfSrcExtract("services.pdf.toImage"), pdfToImageOp),
		"toText":  scriptengine.PromisifyAsync(vm, loop, pdfSrcExtract("services.pdf.toText"), pdfToTextOp),
		"toHtml":  scriptengine.PromisifyAsync(vm, loop, pdfSrcExtract("services.pdf.toHtml"), pdfToHTMLOp),
	}
}
