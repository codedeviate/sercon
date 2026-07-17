//go:build !windows

package main

import (
	"context"
	"testing"
	"time"
)

// In the test process stdin is not a TTY, so execInteractive takes the inherit
// path (no pty/raw mode). That still lets us assert exit-code plumbing and
// timeout behaviour end to end.

func TestExecInteractive_Success(t *testing.T) {
	res, err := execInteractive(context.Background(), execInteractiveArgs{
		argv: []string{"/bin/sh", "-c", "exit 0"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["exitCode"].(int) != 0 || res["success"].(bool) != true {
		t.Fatalf("got %+v, want exitCode 0 / success true", res)
	}
	if _, ok := res["durationMs"].(int64); !ok {
		t.Fatalf("durationMs missing/typed wrong: %+v", res)
	}
}

func TestExecInteractive_NonZeroExit(t *testing.T) {
	res, err := execInteractive(context.Background(), execInteractiveArgs{
		argv: []string{"/bin/sh", "-c", "exit 3"},
	})
	if err != nil {
		t.Fatalf("a non-zero exit must resolve, not throw: %v", err)
	}
	if res["exitCode"].(int) != 3 || res["success"].(bool) != false {
		t.Fatalf("got %+v, want exitCode 3 / success false", res)
	}
}

func TestExecInteractive_Timeout(t *testing.T) {
	_, err := execInteractive(context.Background(), execInteractiveArgs{
		argv:    []string{"/bin/sh", "-c", "sleep 5"},
		timeout: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

// A cancelled run context tears the child down (mirrors a Run deadline).
func TestExecInteractive_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := execInteractive(ctx, execInteractiveArgs{
		argv: []string{"/bin/sh", "-c", "sleep 5"},
	})
	if err == nil {
		t.Fatal("expected a cancellation error, got nil")
	}
}
