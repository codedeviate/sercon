package scriptengine_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// NOTE: the engine sets goja.TagFieldNameMapper("json", true), so Go METHODS
// are exposed UNCAPITALIZED in JS (Greet → greet) and struct fields are only
// exposed when they carry a json tag. The tests therefore call `.greet()` /
// `.value()` (methods), not capitalized names or untagged fields.

type greeter struct{ name string }

func (g *greeter) Greet() string { return "hello " + g.name }

func newGreeter(name string) *greeter { return &greeter{name: name} }

type widget struct{ size int }

func (w *widget) Value() int { return w.size }

func newWidget(size int) (*widget, error) {
	if size < 0 {
		return nil, errors.New("negative size")
	}
	return &widget{size: size}, nil
}

// TestConstructor_NewAndMethod: new Greeter("world") constructs and the Go
// method dispatches (as `greet`); value surfaced via export default (Task 1).
func TestConstructor_NewAndMethod(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterConstructor("Greeter", newGreeter); err != nil {
		t.Fatal(err)
	}
	val, err := eng.Run(context.Background(), "main.ts",
		"export default new Greeter(\"world\").greet();\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == nil || val.String() != "hello world" {
		t.Errorf("expected \"hello world\", got %v", val)
	}
}

// TestConstructor_ErrorResultThrows: a ctor returning a non-nil error makes
// `new` throw.
func TestConstructor_ErrorResultThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterConstructor("Widget", newWidget); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "main.ts", "new Widget(-1);\n")
	if err == nil || !strings.Contains(err.Error(), "negative size") {
		t.Errorf("expected 'negative size' throw, got %v", err)
	}
}

// TestConstructor_ErrorResultOK: same ctor with a valid arg constructs fine;
// the Go method (value) is reachable.
func TestConstructor_ErrorResultOK(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterConstructor("Widget", newWidget); err != nil {
		t.Fatal(err)
	}
	val, err := eng.Run(context.Background(), "main.ts",
		"export default new Widget(5).value();\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == nil || val.ToInteger() != 5 {
		t.Errorf("expected 5, got %v", val)
	}
}
