// cmd/sercon/exttool.go
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultToolMaxOutput = 64 << 20 // 64 MiB cap on buffered stdout
	toolStderrCap        = 500      // bytes of stderr surfaced in an error
)

// toolSpec is one external-CLI invocation: a fixed binary, explicit argv, and
// limits. argv must already be validated by the caller (no shell is used).
type toolSpec struct {
	bin            string
	argv           []string
	timeout        time.Duration // 0 → no explicit timeout (caller should set one)
	maxOutput      int           // 0 → defaultToolMaxOutput
	installHint    string        // surfaced when bin is not on PATH
	combinedOutput bool          // capture stdout+stderr together (for tools that print to stderr, e.g. `-v` version probes)
	capHint        string        // appended to the output-overflow error (e.g. how to redirect to a file); generic message when empty
}

// toolAvailable reports whether bin is resolvable on PATH.
func toolAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// safePathArgs appends caller-supplied file paths after a "--" separator so a
// path beginning with "-" cannot be parsed as a flag. With no paths, flags are
// returned unchanged.
func safePathArgs(flags []string, paths ...string) []string {
	if len(paths) == 0 {
		return flags
	}
	out := make([]string, 0, len(flags)+1+len(paths))
	out = append(out, flags...)
	out = append(out, "--")
	out = append(out, paths...)
	return out
}

// runTool runs `bin argv...` with no shell, applying a timeout and an
// output-size cap. A missing binary, non-zero exit (with trimmed stderr), or
// context cancellation map to a clean error.
func runTool(ctx context.Context, spec toolSpec) ([]byte, error) {
	if !toolAvailable(spec.bin) {
		hint := spec.installHint
		if hint != "" {
			hint = " (" + hint + ")"
		}
		return nil, fmt.Errorf("%s not found on PATH%s", spec.bin, hint)
	}
	runCtx := ctx
	if spec.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, spec.timeout)
		defer cancel()
	}
	outCap := spec.maxOutput
	if outCap <= 0 {
		outCap = defaultToolMaxOutput
	}
	cmd := exec.CommandContext(runCtx, spec.bin, spec.argv...) //nolint:gosec // fixed binary + validated argv, no shell
	var stdout cappedBuffer
	stdout.limit = outCap
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	if spec.combinedOutput {
		cmd.Stderr = &stdout
	} else {
		cmd.Stderr = &stderr
	}
	err := cmd.Run()
	// Surface a cancelled/timed-out run first: a timeout that coincides with
	// large output must report the timeout, not the overflow.
	if runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	if stdout.overflow {
		msg := fmt.Sprintf("%s produced more than %d bytes of output", spec.bin, outCap)
		if spec.capHint != "" {
			msg += "; " + spec.capHint
		}
		return nil, errors.New(msg)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && spec.combinedOutput {
			msg = strings.TrimSpace(stdout.String())
		}
		if len(msg) > toolStderrCap {
			msg = msg[:toolStderrCap]
		}
		if msg != "" {
			return nil, fmt.Errorf("%s failed: %w: %s", spec.bin, err, msg)
		}
		return nil, fmt.Errorf("%s failed: %w", spec.bin, err)
	}
	return stdout.Bytes(), nil
}

// cappedBuffer is a bytes.Buffer that stops accepting data past `limit` and
// records that an overflow happened (so we can fail loudly rather than OOM).
type cappedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.overflow {
		return len(p), nil
	}
	if c.Len()+len(p) > c.limit {
		c.overflow = true
		remain := c.limit - c.Len()
		if remain > 0 {
			// Explicit c.Buffer.Write avoids recursing into this override.
			_, _ = c.Buffer.Write(p[:remain])
		}
		return len(p), nil
	}
	return c.Buffer.Write(p)
}
