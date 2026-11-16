package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// stuffrRunReq describes one invocation of the embedded stuffr module.
type stuffrRunReq struct {
	Argv   []string
	Cwd    string // absolute; caller resolves
	Stdin  []byte
	Env    map[string]string
	Stream bool // tee stderr to os.Stderr
}

// stuffrRunResult is the outcome of a run that reached a guest exit.
type stuffrRunResult struct {
	Code           int
	Stdout, Stderr []byte
}

// stuffrTrapError reports a wasm trap (panic=abort, allocation failure, ...).
type stuffrTrapError struct {
	Argv []string
	Err  error
}

func (e *stuffrTrapError) argv0() string {
	if len(e.Argv) > 0 {
		return e.Argv[0]
	}
	return ""
}

func (e *stuffrTrapError) detail() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *stuffrTrapError) Error() string {
	return fmt.Sprintf("stuffr %s trapped (corrupt or hostile input, memory exhaustion, or a stuffr bug?): %s", e.argv0(), e.detail())
}

func (e *stuffrTrapError) Unwrap() error { return e.Err }

// ErrorFields exposes structured fields on the thrown JS error.
func (e *stuffrTrapError) ErrorFields() map[string]any {
	return map[string]any{"kind": "trap", "argv": e.Argv, "detail": e.detail()}
}

// stuffrRuntime holds a wazero runtime with the stuffr module compiled once.
type stuffrRuntime struct {
	rt       wazero.Runtime
	compiled wazero.CompiledModule
}

// stuffrCacheDir returns the wazero compilation cache dir, "" if unavailable.
var stuffrCacheDir = func() string {
	d, err := os.UserCacheDir()
	if err != nil || d == "" {
		return ""
	}
	return filepath.Join(d, "sercon", "wazero")
}

// usableCacheDir reports whether dir can be created and written to.
func usableCacheDir(dir string) bool {
	if dir == "" {
		return false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_, werr := f.Write([]byte("x"))
	cerr := f.Close()
	_ = os.Remove(name)
	return werr == nil && cerr == nil
}

func newStuffrRuntime(ctx context.Context, cacheDir string) (*stuffrRuntime, error) {
	if len(stuffrPersonalities()) == 0 {
		return nil, errors.New("embedded version.json lists no personalities")
	}
	wasm, err := stuffrWasm()
	if err != nil {
		return nil, fmt.Errorf("stuffr: decompress embedded module: %w", err)
	}
	cfg := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	if usableCacheDir(cacheDir) {
		if cache, cerr := wazero.NewCompilationCacheWithDir(cacheDir); cerr == nil {
			cfg = cfg.WithCompilationCache(cache)
		}
	}
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("stuffr: instantiate wasi: %w", err)
	}
	compiled, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("stuffr: compile module: %w", err)
	}
	return &stuffrRuntime{rt: rt, compiled: compiled}, nil
}

func (r *stuffrRuntime) run(ctx context.Context, req stuffrRunReq) (stuffrRunResult, error) {
	if len(req.Argv) == 0 {
		return stuffrRunResult{}, errors.New("stuffr.run: argv must not be empty")
	}
	argv := req.Argv
	if !stuffrPersonalities()[argv[0]] {
		argv = append([]string{"stuffr"}, argv...)
	}
	var stdout, stderr bytes.Buffer
	var errW io.Writer = &stderr
	if req.Stream {
		errW = io.MultiWriter(&stderr, os.Stderr)
	}
	cfg := wazero.NewModuleConfig().
		WithName("").
		WithArgs(argv...).
		WithStdin(bytes.NewReader(req.Stdin)).
		WithStdout(&stdout).
		WithStderr(errW).
		WithFSConfig(wazero.NewFSConfig().WithDirMount("/", "/")).
		WithSysWalltime().
		WithSysNanotime().
		WithRandSource(rand.Reader).
		WithEnv("STUFFR_CWD", req.Cwd).
		WithEnv("NO_COLOR", "1").
		WithEnv("TERM", "dumb")
	for k, v := range req.Env {
		cfg = cfg.WithEnv(k, v)
	}
	mod, err := r.rt.InstantiateModule(ctx, r.compiled, cfg)
	if mod != nil {
		_ = mod.Close(ctx)
	}
	if err == nil {
		return stuffrRunResult{Code: 0, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, nil
	}
	code, out := classifyStuffrErr(ctx, argv, err)
	if out != nil {
		return stuffrRunResult{}, out
	}
	return stuffrRunResult{Code: code, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, nil
}

// classifyStuffrErr maps an instantiate error to an exit code or a typed error.
func classifyStuffrErr(ctx context.Context, argv []string, err error) (int, error) {
	var ee *sys.ExitError
	if errors.As(err, &ee) {
		if c := ee.ExitCode(); (c == sys.ExitCodeContextCanceled || c == sys.ExitCodeDeadlineExceeded) && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return int(ee.ExitCode()), nil
	}
	if cerr := ctx.Err(); cerr != nil {
		return 0, cerr
	}
	return 0, &stuffrTrapError{Argv: argv, Err: err}
}

var (
	stuffrSharedOnce sync.Once
	stuffrShared     *stuffrRuntime
	stuffrSharedErr  error
)

// sharedStuffrRuntime lazily builds the process-wide runtime.
func sharedStuffrRuntime() (*stuffrRuntime, error) {
	stuffrSharedOnce.Do(func() {
		stuffrShared, stuffrSharedErr = newStuffrRuntime(context.Background(), stuffrCacheDir())
	})
	return stuffrShared, stuffrSharedErr
}
