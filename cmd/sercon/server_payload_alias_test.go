package main

import (
	"testing"

	"github.com/dop251/goja"
)

// snapshotPayload (and the ws.send / res.bytes byte paths that use the same
// idiom) must COPY a Uint8Array's bytes, not alias its backing store. goja's
// Uint8Array export returns a slice over the live ArrayBuffer, so aliasing
// lets a later script mutation change bytes already handed to an off-loop
// writer — a data race and wrong-bytes-on-the-wire. This is deterministic:
// mutate the source after snapshotting and check the snapshot is unchanged.
func TestSnapshotPayload_CopiesNotAliases(t *testing.T) {
	vm := goja.New()
	if _, err := vm.RunString(`var a = new Uint8Array([1, 2, 3])`); err != nil {
		t.Fatal(err)
	}
	snap := snapshotPayload(vm.Get("a"))
	if len(snap) != 3 || snap[0] != 1 {
		t.Fatalf("unexpected snapshot %v", snap)
	}
	// Mutate the original Uint8Array; the snapshot must not change.
	if _, err := vm.RunString(`a[0] = 99`); err != nil {
		t.Fatal(err)
	}
	if snap[0] != 1 {
		t.Fatalf("snapshotPayload aliased the backing array: snap[0]=%d after a[0]=99 (must be 1)", snap[0])
	}
}
