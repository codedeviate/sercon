package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func TestServerHTTP_Static(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("static body"), 0644); err != nil {
		t.Fatal(err)
	}
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
    "GET /assets/{rest...}": server.http.static({dir: "` + dir + `", stripPrefix: "/assets/"}),
  },
});
const r = await net.http.get("http://127.0.0.1:` + strconv.Itoa(port) + `/assets/hello.txt");
if (r.status !== 200) throw new Error("status: " + r.status);
if (r.body !== "static body") throw new Error("body: " + r.body);
await srv.close();
`
	if _, err := eng.Run(context.Background(), "static.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}
