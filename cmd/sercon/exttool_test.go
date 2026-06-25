// cmd/sercon/exttool_test.go
package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSafePathArgs(t *testing.T) {
	got := safePathArgs([]string{"-png", "-f", "1"}, "-weird.pdf", "out")
	want := []string{"-png", "-f", "1", "--", "-weird.pdf", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safePathArgs = %v, want %v", got, want)
	}
	// No paths → no separator appended.
	if g := safePathArgs([]string{"--version"}); !reflect.DeepEqual(g, []string{"--version"}) {
		t.Fatalf("safePathArgs(no paths) = %v, want [--version]", g)
	}
}

func TestToolAvailable_AbsentBinary(t *testing.T) {
	if toolAvailable("sercon-definitely-not-a-real-binary-xyz") {
		t.Fatal("expected absent binary to be unavailable")
	}
}

func TestRunTool_MissingBinaryGivesHint(t *testing.T) {
	_, err := runTool(context.Background(), toolSpec{
		bin: "sercon-definitely-not-a-real-binary-xyz", argv: []string{"--version"},
		installHint: "install the thing",
	})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "install the thing") {
		t.Fatalf("error %q should contain install hint", err)
	}
}

func TestRunTool_NonzeroExitIncludesStderr(t *testing.T) {
	if !toolAvailable("sh") {
		t.Skip("no `sh` on PATH")
	}
	_, err := runTool(context.Background(), toolSpec{
		bin: "sh", argv: []string{"-c", "echo boom 1>&2; exit 1"}, timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected non-zero exit to error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q should include stderr 'boom'", err)
	}
}
