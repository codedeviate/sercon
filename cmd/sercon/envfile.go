package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dop251/goja"
)

// envfile.go implements optional, explicit `.env` loading for the
// `--env-file` flag. It replaces the `set -a; source .env; set +a` ritual:
// values are applied to the process environment (so runtime.env.get and any
// spawned subprocess see them) BEFORE the script runs, and a variable already
// present in the real environment always wins.

// stringSliceFlag is a repeatable string flag (each --env-file appends).
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ", ") }

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// envKV is one parsed assignment from a .env file.
type envKV struct {
	key string
	val string
}

// parseEnvFile parses dotenv-style content: KEY=VALUE per line, `#` comments,
// blank lines, an optional leading `export `, and optional surrounding single
// or double quotes around the value. No shell expansion is performed. A line
// without `=` (or with an empty key) is a hard error reported with its number.
func parseEnvFile(data []byte) ([]envKV, error) {
	var out []envKV
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", line)
		}
		key := strings.TrimSpace(raw[:eq])
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", line)
		}
		val := strings.TrimSpace(raw[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		out = append(out, envKV{key: key, val: val})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// envLoad reads path, parses it with parseEnvFile, and applies each pair to the
// process environment. When override is false an already-set variable is left
// untouched (the --env-file rule). Returns the parsed pairs (all of them,
// regardless of whether each was applied).
func envLoad(path string, override bool) (map[string]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // script-chosen path is intentional
	if err != nil {
		return nil, err
	}
	kvs, err := parseEnvFile(data)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		out[kv.key] = kv.val
		if !override {
			if _, set := os.LookupEnv(kv.key); set {
				continue
			}
		}
		if err := os.Setenv(kv.key, kv.val); err != nil {
			return nil, fmt.Errorf("set %s: %w", kv.key, err)
		}
	}
	return out, nil
}

// envLoadBinding builds the runtime.env.load async handler. It reads arg 0
// (path) and arg 1 ({ override? }), then calls envLoad. Args are read via
// Export() only (no vm method calls) so it is safe in the PromisifyAsync
// goroutine.
func envLoadBinding(_ *goja.Runtime) func(context.Context, goja.FunctionCall) (map[string]any, error) {
	return func(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
		path, ok := call.Argument(0).Export().(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("runtime.env.load: path is required")
		}
		override := false
		if o, ok := call.Argument(1).Export().(map[string]any); ok {
			if b, ok := o["override"].(bool); ok {
				override = b
			}
		}
		m, err := envLoad(path, override)
		if err != nil {
			return nil, fmt.Errorf("runtime.env.load: %w", err)
		}
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out, nil
	}
}

// applyEnvFiles loads each path in order and sets its variables on the process
// environment. A variable already present in the real environment at call time
// always wins (it is never overridden); among files, a later file overrides an
// earlier one for keys not in the real environment.
func applyEnvFiles(paths []string) error {
	real := map[string]bool{}
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i > 0 {
			real[e[:i]] = true
		}
	}
	for _, p := range paths {
		data, err := os.ReadFile(p) //nolint:gosec // user-provided env-file path is intentional
		if err != nil {
			return fmt.Errorf("--env-file %s: %w", p, err)
		}
		kvs, err := parseEnvFile(data)
		if err != nil {
			return fmt.Errorf("--env-file %s: %w", p, err)
		}
		for _, kv := range kvs {
			if real[kv.key] {
				continue // real environment wins
			}
			if err := os.Setenv(kv.key, kv.val); err != nil {
				return fmt.Errorf("--env-file %s: set %s: %w", p, kv.key, err)
			}
		}
	}
	return nil
}
