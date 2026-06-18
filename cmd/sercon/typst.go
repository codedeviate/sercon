package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// typstAvailable reports whether the typst CLI is on PATH.
func typstAvailable() bool {
	_, err := exec.LookPath("typst")
	return err == nil
}

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
	args = append(args, s.inputPath, s.outputPath)
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
	args = append(args, s.inputPath, s.selector)
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
