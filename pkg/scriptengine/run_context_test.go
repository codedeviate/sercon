package scriptengine

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// TestRunContextFromVM verifies the exported helper returns the live Run
// context inside a Run (non-nil, not yet cancelled) and a usable Background
// fallback for a vm that never went through Run.
func TestRunContextFromVM(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var duringNonNil, duringDone bool
	if err := eng.RegisterFactory("grab", func(vm *goja.Runtime, loop *eventloop.EventLoop) any {
		return func() {
			c := RunContextFromVM(vm)
			duringNonNil = c != nil
			if c != nil {
				select {
				case <-c.Done():
					duringDone = true
				default:
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "grab.ts", `grab();`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !duringNonNil {
		t.Fatal("RunContextFromVM returned nil during a Run")
	}
	if duringDone {
		t.Fatal("Run context was already Done() during the Run")
	}

	// Outside a Run: a fresh vm yields a usable (non-nil, not-cancelled) context.
	c := RunContextFromVM(goja.New())
	if c == nil {
		t.Fatal("RunContextFromVM returned nil outside a Run")
	}
	select {
	case <-c.Done():
		t.Fatal("the Background fallback must not be Done()")
	default:
	}
}
