//go:build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// installStdoutRedirect points the process's stdout file descriptor at stderr
// and returns (a) an *os.File bound to the ORIGINAL stdout (for the caller to
// hand to the MCP JSON-RPC transport) and (b) a restore func that puts fd 1
// back the way it was.
//
// Why an fd-level redirect and not a plain `os.Stdout = os.Stderr` swap:
// goja_nodejs's console module binds its stdout logger to `os.Stdout` ONCE at
// package-init time (console/std_printer.go: `stdoutLogger = log.New(os.Stdout,
// …)`), so reassigning the `os.Stdout` variable later does not move
// `console.log` — it keeps writing to the fd the logger already captured
// (fd 1). Remapping fd 1 itself (dup2) is therefore the only lever that moves
// the already-captured logger, plus `runtime.log`'s `fmt.Println` and any stray
// write, all onto stderr in one shot. JSON-RPC then goes out the saved real
// stdout via the returned *os.File, keeping stdout a pure JSON-RPC stream.
//
// Unix only: unix.Dup2 is available on darwin and every linux arch (on linux it
// is implemented in terms of the dup3 syscall, so arm64/riscv64 — which lack a
// raw dup2 — are covered). The windows build gets a separate stub.
func installStdoutRedirect() (saved *os.File, restore func() error, err error) {
	outFd := int(os.Stdout.Fd())

	// Duplicate the current stdout so we retain a handle to the real stdout
	// even after we overwrite fd 1.
	savedFd, err := unix.Dup(outFd)
	if err != nil {
		return nil, nil, err
	}
	savedFile := os.NewFile(uintptr(savedFd), "mcp:real-stdout")

	// Point fd 1 at stderr. After this, everything that writes to fd 1 —
	// the console logger, fmt.Println, etc. — lands on stderr.
	if err := unix.Dup2(int(os.Stderr.Fd()), outFd); err != nil {
		_ = savedFile.Close()
		return nil, nil, err
	}

	restore = func() error {
		rerr := unix.Dup2(savedFd, outFd)
		_ = savedFile.Close()
		return rerr
	}
	return savedFile, restore, nil
}
