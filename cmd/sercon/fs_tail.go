package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// tailPollInterval is the poll-fallback cadence — covers filesystems/editors
// that emit unreliable fsnotify events, plus truncation/rotation detection.
const tailPollInterval = 1 * time.Second

// tailFrom encodes where a follow begins reading.
type tailFrom struct {
	mode string // "end" (default) | "start" | "lines"
	n    int    // trailing line count when mode == "lines"
}

// parseTailFrom reads the `from` option: "end"|"start" or a positive integer N
// (last N lines). Absent/nil → "end".
func parseTailFrom(opts map[string]any) (tailFrom, error) {
	v, ok := opts["from"]
	if !ok || v == nil {
		return tailFrom{mode: "end"}, nil
	}
	switch t := v.(type) {
	case string:
		switch t {
		case "", "end":
			return tailFrom{mode: "end"}, nil
		case "start":
			return tailFrom{mode: "start"}, nil
		default:
			return tailFrom{}, fmt.Errorf(`from must be "end", "start", or a positive number, got %q`, t)
		}
	case int64:
		return linesFrom(int(t))
	case float64:
		return linesFrom(int(t))
	default:
		return tailFrom{}, fmt.Errorf(`from must be "end", "start", or a positive number`)
	}
}

func linesFrom(n int) (tailFrom, error) {
	if n < 0 {
		return tailFrom{}, fmt.Errorf("from: line count must be >= 0, got %d", n)
	}
	return tailFrom{mode: "lines", n: n}, nil
}

// followFile opens path, seeks per from, and follows appends via fsnotify plus
// a 1s poll fallback. It calls onLine for each complete line (trailing \n and
// \r stripped), surviving copytruncate-style truncation and rename/recreate
// rotation. It returns when ctx is canceled or onLine returns an error, and
// returns an error immediately if path does not exist.
func followFile(ctx context.Context, path string, from tailFrom, onLine func(string) error) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("fs.tail: %w", err)
	}
	f, info, ok := openFileStat(abs)
	if !ok {
		return fmt.Errorf("fs.tail: %w", &os.PathError{Op: "open", Path: abs, Err: os.ErrNotExist})
	}
	defer func() { _ = f.Close() }()

	var offset int64
	switch from.mode {
	case "start":
		offset = 0
	case "lines":
		offset, err = seekLastNLines(f, info.Size(), from.n)
		if err != nil {
			return fmt.Errorf("fs.tail: %w", err)
		}
	default: // "end"
		offset = info.Size()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fs.tail: %w", err)
	}
	defer func() { _ = watcher.Close() }()
	// Watch the parent dir (catches Create of a rotated replacement) and the
	// file itself. Either Add may fail transiently; the other + poll cover it.
	_ = watcher.Add(filepath.Dir(abs))
	_ = watcher.Add(abs)

	var partial []byte
	drain := func() error {
		_, derr := readLines(f, &offset, &partial, onLine)
		return derr
	}
	// Deliver anything already past the initial offset ("start"/"lines").
	if err := drain(); err != nil {
		return err
	}

	// rotate swaps to a freshly opened file at abs when it is a different inode.
	rotate := func() error {
		nf, ni, ok := openFileStat(abs)
		if !ok {
			return nil // path momentarily gone; a later event/poll retries
		}
		if os.SameFile(info, ni) {
			_ = nf.Close()
			return nil
		}
		_, _ = readLines(f, &offset, &partial, onLine) // flush tail of old file
		_ = f.Close()
		f, info, offset, partial = nf, ni, 0, nil
		_ = watcher.Add(abs)
		return drain()
	}

	ticker := time.NewTicker(tailPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if ev.Name == abs && ev.Op&fsnotify.Write != 0 {
				if err := drain(); err != nil {
					return err
				}
			}
			if ev.Name == abs && ev.Op&(fsnotify.Rename|fsnotify.Remove|fsnotify.Create) != 0 {
				if err := rotate(); err != nil {
					return err
				}
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			// ignore transient watcher errors; the poll fallback still covers us
		case <-ticker.C:
			st, serr := os.Stat(abs)
			switch {
			case serr != nil:
				// path gone (mid-rotation); keep current handle, retry next tick
			case !os.SameFile(info, st):
				if err := rotate(); err != nil {
					return err
				}
			case st.Size() < offset:
				offset, partial = 0, nil // truncated in place
			}
			if err := drain(); err != nil {
				return err
			}
		}
	}
}

// openFileStat opens path and stats it; ok=false on any error.
func openFileStat(path string) (*os.File, os.FileInfo, bool) {
	f, err := os.Open(path) //nolint:gosec // scripts choose the follow target
	if err != nil {
		return nil, nil, false
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, false
	}
	return f, info, true
}

// readLines reads from *offset to EOF, appends to *partial, and calls onLine for
// each complete (\n-terminated) line with the trailing \n and \r stripped.
// Advances *offset by bytes read and leaves any trailing partial line (compacted
// into a fresh slice) in *partial. Returns bytes read.
func readLines(f *os.File, offset *int64, partial *[]byte, onLine func(string) error) (int, error) {
	if _, err := f.Seek(*offset, io.SeekStart); err != nil {
		return 0, err
	}
	buf := make([]byte, 32*1024)
	total := 0
	acc := *partial
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			total += n
			*offset += int64(n)
			acc = append(acc, buf[:n]...)
			for {
				i := bytes.IndexByte(acc, '\n')
				if i < 0 {
					break
				}
				line := bytes.TrimSuffix(acc[:i], []byte{'\r'})
				if e := onLine(string(line)); e != nil {
					*partial = append([]byte(nil), acc[i+1:]...)
					return total, e
				}
				acc = acc[i+1:]
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			*partial = append([]byte(nil), acc...)
			return total, rerr
		}
	}
	*partial = append([]byte(nil), acc...) // compact trailing partial line
	return total, nil
}

// seekLastNLines returns the byte offset of the start of the last n lines of a
// file of the given size. n<=0 or empty file → EOF (no replay). Reads backward
// in chunks counting '\n', ignoring a single trailing newline at EOF.
func seekLastNLines(f *os.File, size int64, n int) (int64, error) {
	if n <= 0 || size == 0 {
		return size, nil
	}
	const chunk = 32 * 1024
	buf := make([]byte, chunk)
	offset := size
	count := 0
	for offset > 0 {
		read := int64(chunk)
		if offset < read {
			read = offset
		}
		offset -= read
		if _, err := f.ReadAt(buf[:read], offset); err != nil && err != io.EOF {
			return 0, err
		}
		b := buf[:read]
		for i := int(read) - 1; i >= 0; i-- {
			if b[i] != '\n' {
				continue
			}
			if offset+int64(i) == size-1 {
				continue // trailing newline terminates the last line; don't count
			}
			count++
			if count == n {
				return offset + int64(i) + 1, nil
			}
		}
	}
	return 0, nil // fewer than n lines → start of file
}
