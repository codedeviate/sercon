package main

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// freePort returns a TCP port that net.Listen can immediately bind on
// localhost. Used to keep server tests from colliding on a fixed port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestServerHTTP_BasicListenAndRoute(t *testing.T) {
	port := freePort(t)
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
  port: ` + strconv.Itoa(port) + `,
  routes: {
    "GET /": (req, res) => res.text("hello"),
    "GET /json": (req, res) => res.json({ok: true, path: req.path}),
  },
});

// Self-test: make a request to ourselves and verify the response.
const r1 = await net.http.get("http://127.0.0.1:` + strconv.Itoa(port) + `/");
if (r1.status !== 200) throw new Error("status: " + r1.status);
if (r1.body !== "hello") throw new Error("body: " + r1.body);

const r2 = await net.http.get("http://127.0.0.1:` + strconv.Itoa(port) + `/json");
if (r2.status !== 200) throw new Error("json status: " + r2.status);
const data = JSON.parse(r2.body);
if (!data.ok) throw new Error("json ok: " + data.ok);
if (data.path !== "/json") throw new Error("path: " + data.path);

await srv.close();
`
	_, err := eng.Run(context.Background(), "test.ts", script)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestServerHTTP_OnError verifies the optional onError handler renders a
// custom response for both a synchronous throw and an async (Promise)
// rejection, that it sees the error message, and that a route which does NOT
// throw is unaffected.
func TestServerHTTP_OnError(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}

	p := strconv.Itoa(port)
	script := `
const srv = await server.http.listen({
  port: ` + p + `,
  onError: (err, req, res) => {
    res.status(500).json({ handled: true, path: req.path, message: String(err && err.message || err) });
  },
  routes: {
    "GET /boom":  () => { throw new Error("kaboom"); },
    "GET /areject": async () => { throw new Error("async-boom"); },
    "GET /ok":    (req, res) => res.text("fine"),
  },
});

const base = "http://127.0.0.1:` + p + `";

// Sync throw → onError renders custom JSON 500.
const a = await net.http.get(base + "/boom");
if (a.status !== 500) throw new Error("boom status: " + a.status);
const aj = JSON.parse(a.body);
if (!aj.handled || aj.path !== "/boom" || aj.message !== "kaboom")
  throw new Error("boom body: " + a.body);

// Async reject → onError renders custom JSON 500, sees the rejection message.
const b = await net.http.get(base + "/areject");
const bj = JSON.parse(b.body);
if (b.status !== 500 || !bj.handled || bj.message !== "async-boom")
  throw new Error("areject body: " + b.body);

// Non-throwing route is unaffected.
const c = await net.http.get(base + "/ok");
if (c.status !== 200 || c.body !== "fine") throw new Error("ok: " + c.status + " " + c.body);

await srv.close();
`
	if _, err := eng.Run(context.Background(), "test.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestServerHTTP_OnErrorFallback verifies the stock 500 is emitted when an
// onError handler itself throws (so a buggy error handler can't wedge the
// request).
func TestServerHTTP_OnErrorFallback(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}

	p := strconv.Itoa(port)
	script := `
const srv = await server.http.listen({
  port: ` + p + `,
  onError: () => { throw new Error("error handler is itself broken"); },
  routes: { "GET /boom": () => { throw new Error("orig"); } },
});
const r = await net.http.get("http://127.0.0.1:` + p + `/boom");
if (r.status !== 500) throw new Error("expected stock 500, got " + r.status);
await srv.close();
`
	if _, err := eng.Run(context.Background(), "test.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}
