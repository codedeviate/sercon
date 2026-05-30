package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// jsconfigContent is the editor config `sercon init` drops alongside
// sercon.d.ts. A TypeScript language server (VSCode, Zed, Neovim+coc,
// Sublime LSP, …) auto-discovers a sibling jsconfig.json and, with
// sercon.d.ts in the program, gives completion + hover docs for the reserved
// globals in every .ts/.tsx script in the directory — no per-editor plugin.
// moduleResolution "Bundler" lets the extensionless relative imports sercon
// scripts use resolve; types:[] keeps stray @types/* out (sercon isn't Node).
const jsconfigContent = `{
  "compilerOptions": {
    "module": "ESNext",
    "target": "ES2022",
    "lib": ["ES2022"],
    "moduleResolution": "Bundler",
    "allowJs": true,
    "checkJs": false,
    "noEmit": true,
    "types": []
  },
  "include": ["**/*.ts", "**/*.tsx", "**/*.js", "sercon.d.ts"]
}
`

// runInit implements `sercon init [dir]`: it drops sercon.d.ts (the binding
// declarations, same content as `-emit-dts`) and a jsconfig.json into dir
// (default "."), wiring up editor autocomplete with no manual setup. Existing
// files are left untouched unless --force is given.
func runInit(args []string) int {
	fs := flag.NewFlagSet("sercon init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "Overwrite existing sercon.d.ts / jsconfig.json")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sercon init [flags] [dir]")
		fmt.Fprintln(os.Stderr, "  Writes sercon.d.ts + jsconfig.json into dir (default \".\") for editor autocomplete.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "sercon init: at most one directory argument")
		fs.Usage()
		return exitUsage
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "sercon init:", err)
		return exitUsage
	}

	// Emit the declaration file from the same surface the CLI registers.
	eng := scriptengine.New(scriptengine.Options{ProgramName: "sercon", DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		fmt.Fprintln(os.Stderr, "sercon init:", err)
		return exitUsage
	}

	wrote := false
	for _, item := range []struct {
		path string
		fn   func(*os.File) error
	}{
		{filepath.Join(dir, "sercon.d.ts"), func(f *os.File) error { return eng.WriteTypes(f) }},
		{filepath.Join(dir, "jsconfig.json"), func(f *os.File) error { _, e := f.WriteString(jsconfigContent); return e }},
	} {
		ok, err := writeInitFile(item.path, *force, item.fn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sercon init:", err)
			return exitUsage
		}
		if ok {
			fmt.Printf("wrote %s\n", item.path)
			wrote = true
		} else {
			fmt.Printf("skipped %s (exists; use --force)\n", item.path)
		}
	}

	if wrote {
		fmt.Printf("\nEditor autocomplete ready in %s — open a .ts file there in any tsserver\n"+
			"editor (VSCode, Zed, Neovim+coc, Sublime LSP, …). Per-file fallback:\n"+
			"  /// <reference path=\"./sercon.d.ts\" />\n", dir)
	}
	return exitOK
}

// writeInitFile writes via fn unless the path already exists and !force.
// Returns whether it wrote.
func writeInitFile(path string, force bool, fn func(*os.File) error) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	return true, fn(f)
}
