package scriptengine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

type fieldedError struct{ msg string }

func (e fieldedError) Error() string { return e.msg }
func (e fieldedError) ErrorFields() map[string]any {
	return map[string]any{"code": 404, "status": "NOT_FOUND"}
}

func TestPromisifyAsync_RejectAttachesErrorFields(t *testing.T) {
	eng := New(Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := eng.RegisterNamespaceFactory("t", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"boom": PromisifyAsync(vm, loop,
				func(call goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
				func(ctx context.Context, _ struct{}) (any, error) { return nil, fieldedError{"nope"} }),
		}
	}); err != nil {
		t.Fatal(err)
	}
	var out any
	if err := eng.Register("__cap", func(v goja.Value) { out = v.Export() }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "t.ts", `
		t.boom().catch(e => __cap({ msg: e.message, code: e.code, status: e.status }));
	`)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T", out)
	}
	if m["msg"] != "nope" || m["code"] != int64(404) || m["status"] != "NOT_FOUND" {
		t.Fatalf("error fields not attached: %#v", m)
	}
}

func TestPromisifyAsync_RejectPlainErrorUnaffected(t *testing.T) {
	eng := New(Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := eng.RegisterNamespaceFactory("t", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"boom": PromisifyAsync(vm, loop,
				func(call goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
				func(ctx context.Context, _ struct{}) (any, error) { return nil, errors.New("plain boom") }),
		}
	}); err != nil {
		t.Fatal(err)
	}
	var out any
	if err := eng.Register("__cap", func(v goja.Value) { out = v.Export() }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "t.ts", `
		t.boom().catch(e => __cap({ msg: e.message, code: e.code, status: e.status }));
	`)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T", out)
	}
	if m["msg"] != "plain boom" {
		t.Fatalf("expected unchanged message, got %#v", m["msg"])
	}
	if m["code"] != nil || m["status"] != nil {
		t.Fatalf("expected no extra fields attached, got code=%#v status=%#v", m["code"], m["status"])
	}
}
