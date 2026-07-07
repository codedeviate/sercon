package main

import (
	"context"
	"errors"
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

const typstTimeout = 60 * time.Second

// inferTypstFormat maps an output extension to a typst format name.
func inferTypstFormat(path string) (string, error) {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "pdf":
		return "pdf", nil
	case "png":
		return "png", nil
	case "svg":
		return "svg", nil
	default:
		return "", fmt.Errorf("cannot infer format from output path %q", path)
	}
}

// validateTypstInput requires exactly one of input/source.
func validateTypstInput(input, source string) error {
	switch {
	case input == "" && source == "":
		return errors.New("requires `input` (a .typ path) or `source` (inline typst)")
	case input != "" && source != "":
		return errors.New("provide only one of `input` or `source`, not both")
	default:
		return nil
	}
}

// compileSpec is the resolved input to buildCompileArgs (pure).
type compileSpec struct {
	inputPath  string
	outputPath string
	format     string // pdf|png|svg
	root       string
	inputs     map[string]string
	ppi        int
	fontPaths  []string
}

// buildCompileArgs builds the `typst compile …` argv (deterministic order).
func buildCompileArgs(s compileSpec) []string {
	args := []string{"compile"}
	if s.root != "" {
		args = append(args, "--root", s.root)
	}
	for _, fp := range s.fontPaths {
		args = append(args, "--font-path", fp)
	}
	for _, k := range sortedKeys(s.inputs) {
		args = append(args, "--input", k+"="+s.inputs[k])
	}
	args = append(args, "--format", s.format)
	if s.format == "png" && s.ppi > 0 {
		args = append(args, "--ppi", strconv.Itoa(s.ppi))
	}
	// "--" ends option parsing so an inputPath/outputPath starting with "-"
	// (e.g. a file named "-x.typ") can't be mis-parsed as a flag. Mirrors the
	// safePathArgs idiom used for the poppler tools in pdf.go.
	args = append(args, "--", s.inputPath, s.outputPath)
	return args
}

// querySpec is the resolved input to buildQueryArgs (pure).
type querySpec struct {
	inputPath string
	selector  string
	field     string
	one       bool
	root      string
	inputs    map[string]string
}

// buildQueryArgs builds the `typst query …` argv (deterministic order).
func buildQueryArgs(s querySpec) []string {
	args := []string{"query"}
	if s.root != "" {
		args = append(args, "--root", s.root)
	}
	for _, k := range sortedKeys(s.inputs) {
		args = append(args, "--input", k+"="+s.inputs[k])
	}
	if s.field != "" {
		args = append(args, "--field", s.field)
	}
	if s.one {
		args = append(args, "--one")
	}
	// "--" ends option parsing so an inputPath/selector starting with "-"
	// can't be mis-parsed as a flag. Mirrors buildCompileArgs above.
	args = append(args, "--", s.inputPath, s.selector)
	return args
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// optStringSlice reads a []string option (JS string[] → []any).
func optStringSlice(opts map[string]any, key string) []string {
	arr, ok := opts[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// runTypst executes `typst <argv...>` via the shared helper and returns stdout.
func runTypst(ctx context.Context, argv []string) (string, error) {
	out, err := runTool(ctx, toolSpec{
		bin: "typst", argv: argv,
		installHint: "install from https://typst.app or `brew install typst`",
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// typstNoArgsExtract is the extract half for the argument-less typst ops.
func typstNoArgsExtract(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil }

// typstOptsExtract pulls the single opts-object argument out of the JS call
// on the event loop (compile/query are both called as op(opts?)).
func typstOptsExtract(call goja.FunctionCall) (map[string]any, error) {
	opts, _ := firstArgMap(call)
	if opts == nil {
		opts = map[string]any{}
	}
	return opts, nil
}

func typstVersionOp(ctx context.Context, _ struct{}) (any, error) {
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := runTypst(runCtx, []string{"--version"})
	if err != nil {
		return nil, fmt.Errorf("services.typst.version: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func typstFontsOp(ctx context.Context, _ struct{}) (any, error) {
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := runTypst(runCtx, []string{"fonts"})
	if err != nil {
		return nil, fmt.Errorf("services.typst.fonts: %w", err)
	}
	seen := map[string]bool{}
	families := []string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.TrimSpace(line)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		families = append(families, f)
	}
	sort.Strings(families)
	return families, nil
}

// resolveTypstInput validates input/source and returns the input path to use,
// plus a cleanup func (non-nil temp dir when source was written or tmp needed).
func resolveTypstInput(opts map[string]any, needTmpOut bool) (inputPath, tmpDir string, cleanup func(), err error) {
	input := optString(opts, "input", "")
	source := optString(opts, "source", "")
	if verr := validateTypstInput(input, source); verr != nil {
		return "", "", func() {}, verr
	}
	cleanup = func() {}
	if source != "" || needTmpOut {
		d, derr := os.MkdirTemp("", "sercon-typst-*")
		if derr != nil {
			return "", "", func() {}, derr
		}
		tmpDir = d
		cleanup = func() { _ = os.RemoveAll(d) }
	}
	inputPath = input
	if source != "" {
		inputPath = filepath.Join(tmpDir, "main.typ")
		if werr := os.WriteFile(inputPath, []byte(source), 0o600); werr != nil {
			cleanup()
			return "", "", func() {}, werr
		}
	}
	return inputPath, tmpDir, cleanup, nil
}

func typstCompileOp(ctx context.Context, opts map[string]any) (any, error) {
	format := strings.ToLower(optString(opts, "format", ""))
	output := optString(opts, "output", "")
	if format == "" {
		if output != "" {
			f, ferr := inferTypstFormat(output)
			if ferr != nil {
				return nil, fmt.Errorf("services.typst.compile: %w", ferr)
			}
			format = f
		} else {
			format = "pdf"
		}
	}
	if format != "pdf" && format != "png" && format != "svg" {
		return nil, fmt.Errorf("services.typst.compile: invalid format %q (pdf|png|svg)", format)
	}
	if output == "" && format != "pdf" {
		return nil, errors.New("services.typst.compile: png/svg require an output path (use {p} in the path for multi-page docs)")
	}

	returnBytes := output == ""
	inputPath, tmpDir, cleanup, err := resolveTypstInput(opts, returnBytes)
	if err != nil {
		return nil, fmt.Errorf("services.typst.compile: %w", err)
	}
	defer cleanup()

	outputPath := output
	if returnBytes {
		outputPath = filepath.Join(tmpDir, "out.pdf")
	}

	spec := compileSpec{
		inputPath: inputPath, outputPath: outputPath, format: format,
		root: optString(opts, "root", ""), inputs: optStringMap(opts, "inputs"),
		ppi: optInt(opts, "ppi", 0), fontPaths: optStringSlice(opts, "fontPaths"),
	}
	runCtx, cancel := context.WithTimeout(ctx, optMillis(opts, "timeout", typstTimeout))
	defer cancel()
	if _, rerr := runTypst(runCtx, buildCompileArgs(spec)); rerr != nil {
		return nil, fmt.Errorf("services.typst.compile: %w", rerr)
	}
	if returnBytes {
		data, derr := os.ReadFile(outputPath)
		if derr != nil {
			return nil, fmt.Errorf("services.typst.compile: read output: %w", derr)
		}
		return scriptengine.NewOrdered().Set("format", format).Set("bytes", data), nil
	}
	return scriptengine.NewOrdered().Set("format", format).Set("path", output), nil
}

func typstQueryOp(ctx context.Context, opts map[string]any) (any, error) {
	selector := optString(opts, "selector", "")
	if selector == "" {
		return nil, errors.New("services.typst.query: `selector` is required (e.g. \"<label>\" or \"heading\")")
	}
	inputPath, _, cleanup, err := resolveTypstInput(opts, false)
	if err != nil {
		return nil, fmt.Errorf("services.typst.query: %w", err)
	}
	defer cleanup()

	spec := querySpec{
		inputPath: inputPath, selector: selector,
		field: optString(opts, "field", ""), one: optBool(opts, "one", false),
		root: optString(opts, "root", ""), inputs: optStringMap(opts, "inputs"),
	}
	runCtx, cancel := context.WithTimeout(ctx, optMillis(opts, "timeout", typstTimeout))
	defer cancel()
	out, rerr := runTypst(runCtx, buildQueryArgs(spec))
	if rerr != nil {
		return nil, fmt.Errorf("services.typst.query: %w", rerr)
	}
	val, jerr := scriptengine.DecodeOrderedJSON([]byte(out))
	if jerr != nil {
		return nil, fmt.Errorf("services.typst.query: parse JSON: %w", jerr)
	}
	return val, nil
}

// typstNamespace builds the services.typst member map.
func typstNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"available": toolAvailable("typst"),
		"version":   scriptengine.PromisifyAsync(vm, loop, typstNoArgsExtract, typstVersionOp),
		"fonts":     scriptengine.PromisifyAsync(vm, loop, typstNoArgsExtract, typstFontsOp),
		"compile":   scriptengine.PromisifyAsync(vm, loop, typstOptsExtract, typstCompileOp),
		"query":     scriptengine.PromisifyAsync(vm, loop, typstOptsExtract, typstQueryOp),
	}
}
