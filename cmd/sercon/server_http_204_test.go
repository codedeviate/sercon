package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// A route handler that returns without calling any terminal (res.json/text/
// html/bytes/empty/redirect) must produce the documented 204 No Content,
// not 200. A handler that DOES terminate must keep its status.
func TestServerHTTP_UnfinalizedHandlerReturns204(t *testing.T) {
	p := strconv.Itoa(freePort(t))
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	script := `
const srv = await server.http.listen({
  port: ` + p + `,
  routes: {
    "GET /empty": (req, res) => {},                 // no terminal -> 204
    "GET /text":  (req, res) => res.text("hi"),     // terminal -> 200
  },
});
const base = "http://127.0.0.1:` + p + `";
const e = await net.http.get(base + "/empty");
if (e.status !== 204) throw new Error("empty handler: expected 204, got " + e.status);
if (e.body !== "") throw new Error("204 must have empty body, got " + JSON.stringify(e.body));
const t = await net.http.get(base + "/text");
if (t.status !== 200 || t.body !== "hi") throw new Error("text handler: " + t.status + " " + t.body);
await srv.close();
`
	if _, err := eng.Run(context.Background(), "test.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}
