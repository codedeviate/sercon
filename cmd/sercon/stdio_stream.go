package main

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// destKind enumerates where one stack entry sends bytes.
type destKind int

const (
	destStream   destKind = iota // a process stream (os.Stdout / os.Stderr)
	destNull                     // discard
	destFile                     // an owned *os.File
	destCallback                 // a JS line handler (see stdio_callback.go)
	destBuffer                   // an in-memory sink: capture() and tests
)

// destination is one entry on a stream's stack.
type destination struct {
	kind   destKind
	name   string        // destStream: "stdout" | "stderr"
	w      io.Writer     // destStream / destFile / destBuffer sink
	file   *os.File      // destFile: owned by this entry, closed when popped
	path   string        // destFile: for targetInfo and error messages
	append bool          // destFile: opened with O_APPEND
	cb     *lineCallback // destCallback
	tee    bool          // also write to the destination beneath this one
	id     uint64        // identity for out-of-order pop
	failed bool          // a write error has already been reported for this entry
}

// stream is one output stream. It is the stable io.Writer every consumer holds;
// redirects move its destinations underneath it, so nothing has to be
// re-plumbed when a script redirects mid-run.
//
// A single Mutex guards both writes and mutations — deliberately not an RWMutex
// with read-locked writes, because a write is not read-only: it mutates
// destination state (a capture buffer, the callback's partial line, the failed
// flag). Full serialisation also means two goroutines can never interleave
// halves of a line.
type stream struct {
	mu     sync.Mutex
	base   destination
	stack  []destination
	nextID uint64
}

func newStream(name string, w io.Writer) *stream {
	return &stream{base: destination{kind: destStream, name: name, w: w}}
}

// Write sends p to the effective destination, descending through tee entries.
func (s *stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeAt(len(s.stack), p)
	return len(p), nil
}

// writeAt writes to stack level i, where i == 0 means the base destination.
// Called with s.mu held; recurses downward for tee and for fall-through.
func (s *stream) writeAt(i int, p []byte) {
	if i <= 0 {
		s.writeDest(&s.base, 0, p)
		return
	}
	d := &s.stack[i-1]
	s.writeDest(d, i, p)
	if d.tee {
		s.writeAt(i-1, p)
	}
}

// writeDest dispatches one destination. level is the entry's stack level so a
// fall-through can target the destination beneath it.
func (s *stream) writeDest(d *destination, level int, p []byte) {
	switch d.kind {
	case destNull:
		return
	case destCallback:
		// tryFeed returns false when the callback cannot take the bytes
		// (re-entrant write, or the queue is full). Fall through to the
		// destination beneath rather than blocking or dropping.
		if !d.cb.tryFeed(p) {
			s.writeAt(level-1, p)
		}
		return
	case destStream, destFile, destBuffer:
		if _, err := d.w.Write(p); err != nil {
			s.failover(d, level, err, p)
		}
	default:
		// Defensive: an unhandled kind must not nil-deref d.w. Treat it as a
		// no-op rather than guessing at a writer that was never set up for it.
	}
}

// failover reports a write error once per destination, then routes this and
// every later write to the destination beneath. A console.log buried in a
// library is the wrong place to surface a full disk, so this never throws.
func (s *stream) failover(d *destination, level int, err error, p []byte) {
	if !d.failed {
		d.failed = true
		where := d.path
		if where == "" {
			where = d.name
		}
		fmt.Fprintf(os.Stderr, "sercon: %s redirect to %s failed: %v\n", s.base.name, where, err)
	}
	d.kind = destNull // stop retrying this entry
	s.writeAt(level-1, p)
}

// push adds d as the new effective destination and returns an idempotent
// restore. The restore removes THIS entry wherever it has ended up in the
// stack, so nested and out-of-order restores both behave.
func (s *stream) push(d destination) (restore func()) {
	s.mu.Lock()
	s.nextID++
	d.id = s.nextID
	id := d.id
	s.stack = append(s.stack, d)
	s.mu.Unlock()

	var once sync.Once
	return func() { once.Do(func() { s.pop(id) }) }
}

func (s *stream) pop(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.stack {
		if s.stack[i].id != id {
			continue
		}
		s.closeDest(i)
		s.stack = append(s.stack[:i], s.stack[i+1:]...)
		return
	}
}

// reset drops every redirect and releases the resources they held.
func (s *stream) reset() { s.truncateTo(0) }

// truncateTo pops entries until the stack is n deep. reset() is truncateTo(0);
// a nonzero n drops everything pushed after some earlier point while keeping
// whatever was already on the stack at that depth (a saved-depth "scope").
func (s *stream) truncateTo(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.stack) - 1; i >= n; i-- {
		s.closeDest(i)
		s.stack = s.stack[:i]
	}
}

// closeDest releases entry i's resources. Called with s.mu held.
func (s *stream) closeDest(i int) {
	d := &s.stack[i]
	switch d.kind {
	case destFile:
		if d.file != nil {
			_ = d.file.Close()
			d.file = nil
		}
	case destCallback:
		// A trailing partial line goes to the destination BENEATH the
		// callback, not to the callback: that is deterministic and needs no
		// live event loop, so nothing is lost when a script exits mid-line.
		//
		// takePartial() and stop() are called here with s.mu held (closeDest
		// is only ever reached from pop()/reset(), both locked). The real
		// lineCallback (Task 4) must not re-enter the stream (no Write/push/
		// pop back onto s) or block waiting on the event loop from either
		// method, or this deadlocks.
		if rest := d.cb.takePartial(); len(rest) > 0 {
			s.writeAt(i, append(rest, '\n'))
		}
		d.cb.stop()
	}
}

// targetInfo describes the effective destination for runtime.<stream>.target().
func (s *stream) targetInfo() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.base
	if n := len(s.stack); n > 0 {
		d = &s.stack[n-1]
	}
	info := map[string]any{"tee": d.tee, "depth": len(s.stack)}
	switch d.kind {
	case destStream:
		info["kind"] = "stream"
		info["name"] = d.name
	case destNull:
		info["kind"] = "null"
	case destFile:
		info["kind"] = "file"
		info["path"] = d.path
		info["append"] = d.append
	case destCallback:
		info["kind"] = "callback"
	case destBuffer:
		info["kind"] = "buffer"
	}
	return info
}

// nullDest discards everything written to it.
func nullDest(tee bool) destination {
	return destination{kind: destNull, tee: tee}
}

// fileDest opens path and returns a destination that owns the file — it is
// closed when the entry is popped or reset. Opening errors surface HERE, at the
// script's own call site, rather than at some later write deep in a library.
func fileDest(path string, appendMode, tee bool) (destination, error) {
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return destination{}, err
	}
	return destination{
		kind:   destFile,
		w:      f,
		file:   f,
		path:   path,
		append: appendMode,
		tee:    tee,
	}, nil
}

// processStreamDest folds this stream onto one of the PROCESS streams. It
// deliberately resolves to os.Stdout / os.Stderr rather than to the other
// stream object's current destination: that makes a cycle
// (stdout -> stderr while stderr -> stdout) impossible by construction, and it
// matches what "fold onto stderr" means everywhere else in sercon.
//
// Reads stdioOutStream.base.w / stdioErrStream.base.w without taking either
// stream's mutex: base.w is written once at construction (newStream) and
// never mutated afterward, so this is safe without the lock.
func processStreamDest(name string, tee bool) (destination, error) {
	switch name {
	case "stdout":
		return destination{kind: destStream, name: "stdout", w: stdioOutStream.base.w, tee: tee}, nil
	case "stderr":
		return destination{kind: destStream, name: "stderr", w: stdioErrStream.base.w, tee: tee}, nil
	default:
		return destination{}, fmt.Errorf("unknown stream %q (want \"stdout\" or \"stderr\")", name)
	}
}
