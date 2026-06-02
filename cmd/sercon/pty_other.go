//go:build windows

package main

import (
	"io"
	"os/exec"
)

// startPTY is unsupported on Windows; execShell falls back to pipes.
func startPTY(_ *exec.Cmd) (io.ReadCloser, error) {
	return nil, errPTYUnsupported
}
