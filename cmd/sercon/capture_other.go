//go:build !linux && !darwin

package main

import (
	"fmt"
	"runtime"
)

// openLiveCapture is unsupported off Linux/macOS: pure-Go live capture needs
// AF_PACKET (Linux) or BPF (macOS). Windows would need Npcap (cgo), which this
// build deliberately avoids. The stub keeps release cross-compiles green.
func openLiveCapture(_ string, _ bool, _ int) (liveSource, error) {
	return nil, fmt.Errorf("net.capture.open: live capture not supported on %s (Linux/macOS only)", runtime.GOOS)
}
