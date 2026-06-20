package scriptengine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// extend past the original short timeout → survives.
func TestRunTimeout_Extend(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true, Timeout: 100 * time.Millisecond})
	if err := eng.Register("bump", func() { eng.SetRunTimeout(5 * time.Second) }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "extend.ts", `
		bump();
		await new Promise((r) => setTimeout(r, 250));
	`)
	if err != nil {
		t.Fatalf("expected survival after extend, got %v", err)
	}
}

// add a deadline to an unlimited (Timeout:0) run → busy loop is killed.
func TestRunTimeout_AddToUnlimited(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true, Timeout: 0})
	if err := eng.Register("arm", func() { eng.SetRunTimeout(80 * time.Millisecond) }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "arm.ts", `arm(); while (true) {}`)
	if !errors.Is(err, scriptengine.ErrScriptTimeout) {
		t.Fatalf("expected ErrScriptTimeout, got %v", err)
	}
}

// clear a short deadline → survives a longer sleep.
func TestRunTimeout_Clear(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true, Timeout: 100 * time.Millisecond})
	if err := eng.Register("clear", func() { eng.SetRunTimeout(0) }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "clear.ts", `
		clear();
		await new Promise((r) => setTimeout(r, 250));
	`)
	if err != nil {
		t.Fatalf("expected survival after clear, got %v", err)
	}
}

// RunTimeoutRemaining reports a value after a set and false after clear.
func TestRunTimeoutRemaining(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true, Timeout: 0})
	var afterSet time.Duration
	var setOK, afterClearOK bool
	if err := eng.Register("probe", func() {
		eng.SetRunTimeout(500 * time.Millisecond)
		afterSet, setOK = eng.RunTimeoutRemaining()
		eng.SetRunTimeout(0)
		_, afterClearOK = eng.RunTimeoutRemaining()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "probe.ts", `probe();`); err != nil {
		t.Fatal(err)
	}
	if !setOK || afterSet <= 0 || afterSet > 500*time.Millisecond {
		t.Fatalf("after set: remaining=%v ok=%v want (0,500ms] true", afterSet, setOK)
	}
	if afterClearOK {
		t.Fatalf("after clear: ok=%v want false", afterClearOK)
	}
}
