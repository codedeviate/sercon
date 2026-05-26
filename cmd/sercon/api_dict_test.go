package main

import (
	"bufio"
	"context"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// fakeDICT speaks just enough RFC 2229 for the binding tests: a 220
// banner, CLIENT, DEFINE (one canned definition for "test", 552 for
// anything else), MATCH (two canned matches), and QUIT.
func fakeDICT(t *testing.T) (host, port string) {
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
				w := bufio.NewWriter(c)
				r := bufio.NewReader(c)
				_, _ = w.WriteString("220 fake DICT server <auth.mime> <1@fake>\r\n")
				_ = w.Flush()
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					f := strings.Fields(strings.TrimSpace(line))
					if len(f) == 0 {
						continue
					}
					switch strings.ToUpper(f[0]) {
					case "CLIENT":
						_, _ = w.WriteString("250 ok\r\n")
					case "DEFINE":
						word := strings.Trim(f[len(f)-1], `"`)
						if word == "test" {
							_, _ = w.WriteString("150 1 definitions retrieved\r\n")
							_, _ = w.WriteString(`151 "test" wn "WordNet"` + "\r\n")
							_, _ = w.WriteString("test\r\n  a trial\r\n.\r\n")
							_, _ = w.WriteString("250 ok\r\n")
						} else {
							_, _ = w.WriteString("552 no match\r\n")
						}
					case "MATCH":
						_, _ = w.WriteString("152 2 matches found\r\n")
						_, _ = w.WriteString(`wn "test"` + "\r\n")
						_, _ = w.WriteString(`wn "testing"` + "\r\n")
						_, _ = w.WriteString(".\r\n")
						_, _ = w.WriteString("250 ok\r\n")
					case "QUIT":
						_, _ = w.WriteString("221 bye\r\n")
						_ = w.Flush()
						return
					default:
						_, _ = w.WriteString("500 unknown\r\n")
					}
					_ = w.Flush()
				}
			}(c)
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p
}

func runDictScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("dict", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return dictNamespace(vm, loop)
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
	if _, err := eng.Run(context.Background(), "d.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestDict_Define(t *testing.T) {
	h, p := fakeDICT(t)
	got := runDictScript(t, `
		const r = await dict.define("`+h+`", "test", { port: "`+p+`" });
		const __result = [r.found, r.definitions.length, r.definitions[0].dbName, r.definitions[0].text.includes("trial")].join(",");
	`)
	if got != "true,1,WordNet,true" {
		t.Errorf("define: %v", got)
	}
}

func TestDict_DefineNotFound(t *testing.T) {
	h, p := fakeDICT(t)
	got := runDictScript(t, `
		const r = await dict.define("`+h+`", "zzznotaword", { port: "`+p+`" });
		const __result = [r.found, r.definitions.length].join(",");
	`)
	if got != "false,0" {
		t.Errorf("not-found: %v", got)
	}
}

func TestDict_Match(t *testing.T) {
	h, p := fakeDICT(t)
	got := runDictScript(t, `
		const r = await dict.match("`+h+`", "test", { port: "`+p+`" });
		const __result = r.matches.map(m => m.word).join(",");
	`)
	if got != "test,testing" {
		t.Errorf("match: %v", got)
	}
}

func TestDict_Validation(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("dict", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return dictNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await dict.define("", "w");`); err == nil {
		t.Error("empty host should throw")
	}
}

func TestDictFields(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`wn "test"`, []string{"wn", "test"}},
		{`"foo bar" baz`, []string{"foo bar", "baz"}},
		{`a b c`, []string{"a", "b", "c"}},
		{``, nil},
	}
	for _, tc := range cases {
		if got := dictFields(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("dictFields(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
