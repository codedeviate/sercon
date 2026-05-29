package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runServe handles the `sercon serve` subcommand. Behaves like vanilla
// sercon plus: access log to stderr, --shutdown-timeout (default 30s),
// --port-override, "READY listening on …" lines on stdout.
func runServe(args []string) int {
	fs := flag.NewFlagSet("sercon serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	scriptTimeout := fs.Duration("timeout", 0, "Per-script timeout (0 = disabled — appropriate for long-running servers)")
	root := fs.String("root", "", "Script root for require resolution (default: dirname of first script)")
	shutdown := fs.Duration("shutdown-timeout", 30*time.Second, "Graceful-shutdown deadline on SIGTERM/SIGINT")
	portOverride := fs.Int("port-override", 0, "If non-zero, replace every server.*.listen({port}) value with this port")
	verbose := fs.Bool("v", false, "Verbose engine tracing to stderr")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sercon serve [flags] script.ts [-- args...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	// Userargs after "--"
	remaining := fs.Args()
	var userArgs []string
	for i, a := range remaining {
		if a == "--" {
			userArgs = remaining[i+1:]
			remaining = remaining[:i]
			break
		}
	}
	if len(remaining) != 1 {
		fmt.Fprintln(os.Stderr, "sercon serve: exactly one script required")
		fs.Usage()
		return exitUsage
	}
	scriptPath := remaining[0]

	scriptRoot := *root
	if scriptRoot == "" {
		abs, err := filepath.Abs(scriptPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sercon serve:", err)
			return exitUsage
		}
		scriptRoot = filepath.Dir(abs)
	}

	engOpts := scriptengine.Options{
		Timeout:     *scriptTimeout,
		ScriptRoot:  scriptRoot,
		ProgramName: "sercon",
	}
	if *verbose {
		engOpts.Verbose = os.Stderr
	}
	eng := scriptengine.New(engOpts)
	if err := registerSurface(eng); err != nil {
		fmt.Fprintln(os.Stderr, "sercon serve:", err)
		return exitUsage
	}

	// Install serve-mode hooks. Read by httpListen for port override + readiness;
	// read by dispatchHandler for access logging. Cleared on return via defer
	// so re-running runServe in the same process (e.g. tests) starts clean.
	servePortOverride = *portOverride
	serveAccessLogger = stderrAccessLogger
	serveSMTPLogger = smtpStderrLogger
	serveReadyWriter = os.Stdout
	defer func() {
		servePortOverride = 0
		serveAccessLogger = nil
		serveSMTPLogger = nil
		serveReadyWriter = nil
	}()

	// Signal handling for graceful shutdown.
	var signaledShutdown atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case sig := <-sigCh:
			_ = sig
			signaledShutdown.Store(true)
			// Give the script `shutdown` time to drain before hard-cancelling.
			// The script's HoldRun sentinels keep the loop alive until each
			// listener's .close() fires; if they don't all close within the
			// shutdown window, we force-cancel.
			t := time.NewTimer(*shutdown)
			defer t.Stop()
			select {
			case <-t.C:
			case <-ctx.Done():
			}
			cancel()
		case <-ctx.Done():
			// runServe returning naturally — unblock and exit so we don't leak.
			return
		}
	}()

	_, err := eng.RunFile(ctx, scriptPath, scriptengine.WithArgs(userArgs))
	if err != nil {
		// Clean SIGTERM/SIGINT shutdown: don't print FAIL, return exitOK.
		// The signal goroutine cancelled the context after shutdown-timeout
		// elapsed (or earlier if all listeners closed). The resulting
		// context.Canceled is the documented graceful path.
		if signaledShutdown.Load() && errors.Is(err, context.Canceled) {
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "FAIL %s: %s\n", scriptPath, err)
		return classifyErr(err)
	}
	return exitOK
}

// stderrAccessLogger writes one access-log line per request to stderr.
// Format: timestamp remote method path status dur_us
func stderrAccessLogger(remote, method, path string, status int, dur time.Duration) {
	fmt.Fprintf(os.Stderr, "%s %s %s %s %d %dµs\n",
		time.Now().UTC().Format(time.RFC3339), remote, method, path, status, dur.Microseconds())
}

// smtpStderrLogger writes one per-stage SMTP log line to stderr.
// Format: ts remote STAGE detail ACCEPTED|REJECTED durµs
func smtpStderrLogger(remote, stage, detail string, accepted bool, dur time.Duration) {
	verdict := "ACCEPTED"
	if !accepted {
		verdict = "REJECTED"
	}
	if detail == "" {
		fmt.Fprintf(os.Stderr, "%s %s %s %s %dµs\n",
			time.Now().UTC().Format(time.RFC3339), remote, stage, verdict, dur.Microseconds())
		return
	}
	fmt.Fprintf(os.Stderr, "%s %s %s %s %s %dµs\n",
		time.Now().UTC().Format(time.RFC3339), remote, stage, detail, verdict, dur.Microseconds())
}
