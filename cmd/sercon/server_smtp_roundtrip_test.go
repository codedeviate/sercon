package main

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestServerSMTP_Roundtrip listens on a free port, sends a message to
// itself via net.email.send, and asserts that onData fired with the
// expected subject + body. Exercises server.smtp.listen AND net.email.send.
func TestServerSMTP_Roundtrip(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        10 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var received atomic.Int32
	if err := eng.Register("__receive", func() {
		received.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	script := `
let captured = null;
const srv = await server.smtp.listen({
  port: ` + strconv.Itoa(port) + `,
  hostname: "test.local",
  handlers: {
    onMail: () => true,
    onRcpt: () => true,
    onData: (env, msg) => {
      captured = { subject: msg.subject, body: msg.body.text, from: env.from };
      __receive();
      return true;
    },
  },
});

const result = await net.email.send({
  to: "alice@test.local",
  from: "bob@test.local",
  subject: "round-trip",
  body: "hello smtp",
  server: { host: "127.0.0.1", port: ` + strconv.Itoa(port) + `, tls: "none" },
});

if (result.accepted.length !== 1) throw new Error("expected 1 accepted, got " + result.accepted.length);
if (result.rejected.length !== 0) throw new Error("unexpected rejections: " + JSON.stringify(result.rejected));
if (!captured || captured.subject !== "round-trip") {
  throw new Error("onData did not capture expected message: " + JSON.stringify(captured));
}
if (!captured.body.includes("hello smtp")) {
  throw new Error("body mismatch: " + captured.body);
}

await srv.close();
`
	if _, err := eng.Run(context.Background(), "rt.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
	if received.Load() != 1 {
		t.Fatalf("expected onData to fire once, got %d", received.Load())
	}
}
