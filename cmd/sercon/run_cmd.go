package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runRun implements the `sercon run <script> [args...]` subcommand. Unlike the
// default mode — where every positional is a separate script and script
// arguments must follow a standalone `--` — `run` executes exactly ONE script
// and hands every token after it to that script as runtime.argv[2:].
//
// This is what makes a directly-executable script practical: a file beginning
// with `#!/usr/bin/env -S sercon run` is launched by the kernel as
// `sercon run /abs/path/script.ts user-arg1 user-arg2 …`, so the user args
// land on the script instead of being mistaken for additional scripts (and no
// `--` separator — which a shebang line can't inject — is needed).
//
// Flags (e.g. -timeout, -root, -v) are accepted before the script; Go's flag
// parser stops at the first non-flag token, so everything from the script path
// onward is positional and becomes script args.
func runRun(args []string) int {
	fs := flag.NewFlagSet("sercon run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 10*time.Second, "Per-script timeout")
	root := fs.String("root", "", "Script root for require resolution (default: dirname of the script)")
	verbose := fs.Bool("v", false, "Verbose: trace transpile output and module resolutions to stderr")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sercon run [flags] <script> [args...]")
		fmt.Fprintln(os.Stderr, "  Runs one script; tokens after <script> become runtime.argv[2:].")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "sercon run: no script given")
		fs.Usage()
		return exitUsage
	}
	script := rest[0]
	userArgs := rest[1:]

	scriptRoot := *root
	if scriptRoot == "" && script != "-" {
		abs, err := filepath.Abs(script)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sercon:", err)
			return exitUsage
		}
		scriptRoot = filepath.Dir(abs)
	}

	engOpts := scriptengine.Options{
		Timeout:        *timeout,
		ScriptRoot:     scriptRoot,
		ProgramName:    "sercon",
		DisableConsole: true, // CLI provides its own clean `console` (console.go)
	}
	if *verbose {
		engOpts.Verbose = os.Stderr
	}
	engOpts.ModuleLoader = paymentprovidersLoader(engOpts.ModuleLoader)
	eng := scriptengine.New(engOpts)
	if err := registerSurface(eng); err != nil {
		fmt.Fprintln(os.Stderr, "sercon:", err)
		return exitUsage
	}

	if err := runOne(eng, script, *verbose, userArgs); err != nil {
		label := script
		if script == "-" {
			label = "<stdin>"
		}
		fmt.Printf("FAIL %s: %s\n", label, err)
		return classifyErr(err)
	}
	return exitOK
}
