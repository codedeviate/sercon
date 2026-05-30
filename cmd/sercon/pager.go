package main

import (
	"io"
	"os"
	"os/exec"
)

// Seams for testing: production points them at the real stdout / TTY check.
var (
	pagerStdout  io.Writer = os.Stdout
	pagerIsTTYFn           = func() bool { return isTTY(os.Stdout) }
)

// pageOutput renders long help / examples output through a pager when stdout
// is a terminal — like git. `render` writes the (optionally colorized) output
// to the writer it's given. When stdout is redirected/piped, `--no-pager` is
// set, or PAGER is empty/"cat", it renders straight to stdout instead.
//
// The pager is `$PAGER` (run via `sh -c` so it may carry args, e.g. "less -R")
// falling back to `less`. For `less` we default LESS=FRX (quit-if-one-screen,
// keep ANSI color, no screen-clear). Color is forced for the duration: the
// pipe to the pager isn't a TTY (so the styler would normally disable color),
// but the user is on one and `less -R` renders escapes — NO_COLOR still wins,
// since the styler checks it before FORCE_COLOR.
func pageOutput(noPager bool, render func(w io.Writer)) {
	if noPager || !pagerIsTTYFn() {
		render(pagerStdout)
		return
	}
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}
	if pager == "cat" { // conventional "no pager" sentinel
		render(pagerStdout)
		return
	}

	cmd := exec.Command("sh", "-c", pager)
	cmd.Env = os.Environ()
	if _, ok := os.LookupEnv("LESS"); !ok {
		cmd.Env = append(cmd.Env, "LESS=FRX")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		render(pagerStdout)
		return
	}
	if err := cmd.Start(); err != nil {
		render(pagerStdout)
		return
	}

	restore := forceColorEnv()
	render(stdin)
	restore()
	_ = stdin.Close()
	_ = cmd.Wait()
}

// forceColorEnv sets FORCE_COLOR for the paged render and returns a restore
// func. The styler honors FORCE_COLOR (after NO_COLOR), so colorized help
// survives the non-TTY pipe to the pager.
func forceColorEnv() func() {
	prev, had := os.LookupEnv("FORCE_COLOR")
	_ = os.Setenv("FORCE_COLOR", "1")
	return func() {
		if had {
			_ = os.Setenv("FORCE_COLOR", prev)
		} else {
			_ = os.Unsetenv("FORCE_COLOR")
		}
	}
}
