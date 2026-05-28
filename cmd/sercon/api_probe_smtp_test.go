package main

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// fakeSMTP starts a minimal SMTP listener that emits a greeting and a
// canned EHLO capability list, then closes. Returns the host:port.
func fakeSMTP(t *testing.T) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				_, _ = c.Write([]byte("220 mail.test ESMTP ready\r\n"))
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"):
						_, _ = c.Write([]byte("250-mail.test at your service\r\n" +
							"250-STARTTLS\r\n" +
							"250-AUTH PLAIN LOGIN\r\n" +
							"250-SIZE 35882577\r\n" +
							"250 8BITMIME\r\n"))
					case strings.HasPrefix(cmd, "QUIT"):
						_, _ = c.Write([]byte("221 bye\r\n"))
						return
					default:
						_, _ = c.Write([]byte("500 unknown\r\n"))
					}
				}
			}(c)
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p
}

func runSMTPScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return probeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "s.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestNetSMTP_CapabilityProbe(t *testing.T) {
	host, port := fakeSMTP(t)
	got := runSMTPScript(t, `
		const r = await net.smtp("`+host+`", { port: "`+port+`" });
		const __result = [
			r.banner,
			r.starttls,
			r.authMechanisms.join("+"),
			r.sizeLimit,
			r.extensions.length,
		].join("|");
	`)
	want := "mail.test ESMTP ready|true|PLAIN+LOGIN|35882577|4"
	if got != want {
		t.Errorf("got %v\nwant %s", got, want)
	}
}

func TestNetSMTP_NoStartTLS(t *testing.T) {
	// A server with no STARTTLS in its EHLO reports starttls:false.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				_, _ = c.Write([]byte("220 plain.test\r\n"))
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if strings.HasPrefix(strings.ToUpper(line), "EHLO") {
						_, _ = c.Write([]byte("250-plain.test\r\n250 8BITMIME\r\n"))
					} else {
						_, _ = c.Write([]byte("221 bye\r\n"))
						return
					}
				}
			}(c)
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	got := runSMTPScript(t, `
		const r = await net.smtp("`+h+`", { port: "`+p+`" });
		const __result = [r.starttls, r.authMechanisms.length].join(",");
	`)
	if got != "false,0" {
		t.Errorf("no-starttls: %v (want false,0)", got)
	}
}

func TestNetSMTP_ConnectionRefusedThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return probeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await net.smtp("127.0.0.1", { port: "1", timeout: 500 });`)
	if err == nil {
		t.Fatal("expected dial error")
	}
}
