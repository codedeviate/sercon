package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runScript builds an engine with registerSurface applied, runs script, and
// returns whatever the script wrote via runtime.log (captured from stdout)
// alongside the Run error. Mirrors the engine-construction pattern used in
// server_http_test.go and the stdout-capture pattern in run_output_test.go;
// no helper by this exact name existed yet in cmd/sercon, so it's added here.
func runScript(t *testing.T, script string) (string, error) {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() {
		_, runErr = eng.Run(context.Background(), "test.ts", script)
	})
	return out, runErr
}

func TestMCPServe_RegistersAndValidates(t *testing.T) {
	out, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		runtime.log(typeof srv.tool, typeof srv.resource, typeof srv.prompt, typeof srv.stdio, typeof srv.listen, typeof srv.close);
	`)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if !strings.Contains(out, "function function function function function function") {
		t.Fatalf("handle missing methods: %q", out)
	}
}

func TestMCPServe_BadConfigThrows(t *testing.T) {
	if _, err := runScript(t, `mcp.serve({ version: "1.0.0" });`); err == nil {
		t.Fatal("expected throw for missing name")
	}
	if _, err := runScript(t, `mcp.serve("nope");`); err == nil {
		t.Fatal("expected throw for non-object config")
	}
}
