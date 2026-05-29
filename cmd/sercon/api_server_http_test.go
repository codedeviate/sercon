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
