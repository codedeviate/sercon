package main

import (
	"context"
	"crypto/tls"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestServerHTTPS_SelfSigned brings up an https listener with the
// `cert: "self-signed"` shortcut and completes a real TLS handshake against
// it (skipping verification, as a client of a self-signed cert must), then
// checks the cert's CN and that loopback is in the SANs.
func TestServerHTTPS_SelfSigned(t *testing.T) {
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
const srv = await server.https.listen({
  port: ` + strconv.Itoa(port) + `,
  cert: "self-signed",
  routes: { "GET /": (req, res) => res.text("secure") },
});
// Hold open until the test dials in, then the test closes via context end.
await new Promise(r => runtime.time.sleep(2000).then(r));
await srv.close();
`
	// Run the listener in the background; the script self-closes after 2s.
	done := make(chan error, 1)
	go func() {
		_, err := eng.Run(context.Background(), "test.ts", script)
		done <- err
	}()

	// Poll-dial the TLS port until it accepts (listener binds synchronously,
	// but the goroutine needs a beat to start the Run).
	var state tls.ConnectionState
	deadline := time.Now().Add(3 * time.Second)
	var dialed bool
	for time.Now().Before(deadline) {
		conn, err := tls.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port),
			&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // self-signed dev cert, verification intentionally skipped
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		state = conn.ConnectionState()
		_ = conn.Close()
		dialed = true
		break
	}
	if !dialed {
		t.Fatal("could not complete TLS handshake against self-signed listener")
	}

	if len(state.PeerCertificates) == 0 {
		t.Fatal("no peer certificate presented")
	}
	leaf := state.PeerCertificates[0]
	if leaf.Subject.CommonName != "localhost" {
		t.Errorf("cert CN = %q, want localhost", leaf.Subject.CommonName)
	}
	var hasLocalhost bool
	for _, n := range leaf.DNSNames {
		if n == "localhost" {
			hasLocalhost = true
		}
	}
	if !hasLocalhost {
		t.Errorf("cert DNSNames = %v, want to include localhost", leaf.DNSNames)
	}

	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
}
