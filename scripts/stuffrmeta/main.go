// Command stuffrmeta runs `stuffr version --json` from a stuffr wasm module
// under wazero and writes the JSON to a file: stuffrmeta <wasm> <out>.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: stuffrmeta <wasm> <out>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "stuffrmeta:", err)
		os.Exit(1)
	}
}

func run(wasmPath, out string) error {
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return err
	}
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	var stdout, stderr bytes.Buffer
	cfg := wazero.NewModuleConfig().
		WithArgs("stuffr", "version", "--json").
		WithStdout(&stdout).WithStderr(&stderr)
	if _, err := rt.InstantiateWithConfig(ctx, wasm, cfg); err != nil {
		return fmt.Errorf("running version --json: %w (stderr: %s)", err, stderr.String())
	}
	var v map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		return fmt.Errorf("invalid JSON from version --json: %w", err)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(pretty, '\n'), 0o644)
}
