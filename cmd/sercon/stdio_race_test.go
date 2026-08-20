package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// Writes arrive from arbitrary goroutines (server handlers, exec.stream, async
// ops) while a script pushes and pops redirects. Under the old plain package
// vars this was an unsynchronised read/write; the stream Mutex is what makes it
// safe. Run with -race.
func TestStream_ConcurrentWritesAndRedirects(t *testing.T) {
	var base bytes.Buffer
	s := newStream("stdout", &base)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = s.Write([]byte("line-from-goroutine\n"))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			var sink bytes.Buffer
			restore := s.push(destination{kind: destBuffer, w: &sink})
			restore()
		}
	}()
	wg.Wait()

	// Every line that reached the base must be whole — no interleaved halves.
	for _, ln := range strings.Split(strings.TrimSuffix(base.String(), "\n"), "\n") {
		if ln != "" && ln != "line-from-goroutine" {
			t.Fatalf("interleaved write: %q", ln)
		}
	}
}
