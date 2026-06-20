package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
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
