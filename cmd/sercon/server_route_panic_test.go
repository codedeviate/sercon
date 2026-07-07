package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// A bad or conflicting route pattern must surface as a catchable JS error,
// not an unrecovered Go-error panic that unwinds out of the event loop and
// crashes the whole process. net/http.ServeMux panics with a bare Go error
// for a pattern like "health" (no leading slash) or a {wildcard} conflict.
func TestServerHTTP_InvalidRoutePatternIsCatchable(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	p := strconv.Itoa(freePort(t))
	script := `
let caught = "";
try {
  await server.http.listen({ port: ` + p + `, routes: { "health": (req, res) => res.text("x") } });
} catch (e) {
  caught = String(e);
}
if (!caught) throw new Error("invalid route pattern was not catchable (process would have crashed)");
if (!caught.includes("route")) throw new Error("unexpected error text: " + caught);
`
	if _, err := eng.Run(context.Background(), "test.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}
