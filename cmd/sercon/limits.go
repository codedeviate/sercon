package main

import (
	"fmt"
	"io"
)

// Shared safety caps for parsers that consume untrusted external dumps
// (PHP serialize/var_dump/var_export, Perl Data::Dumper, and friends). These
// exist to turn pathological input (deeply nested structures, oversized
// counts, ...) into a normal returned error instead of a crash (stack
// overflow, OOM, panic).
const (
	// MaxDecodeDepth caps the nesting depth a recursive-descent decoder will
	// follow before giving up with an error. Well below the depth that would
	// exhaust a goroutine stack, but far beyond any realistic well-formed
	// dump.
	MaxDecodeDepth = 10_000

	// DefaultMaxHTTPBodyBytes caps the size of an HTTP response body read
	// into memory by net.http.request and the shared web.* fetch helper
	// (webFetch). Without a cap, a large or slow-drip response body read via
	// io.ReadAll can OOM the process. Callers may override per-call via the
	// `maxBytes` option.
	DefaultMaxHTTPBodyBytes = 256 << 20 // 256 MB

	// DefaultMaxServerBodyBytes caps the size of an inbound request body
	// read into memory by server.http.listen / server.https.listen before
	// a route handler or middleware runs. Without a cap, a large POST body
	// read via io.ReadAll can OOM the process before any JS code sees the
	// request. Listeners may override per-listener via the `maxBodyBytes`
	// option; a request over the cap gets a 413 without invoking JS.
	DefaultMaxServerBodyBytes = 32 << 20 // 32 MB

	// DefaultMaxDecompressBytes caps the decompressed output size read from
	// a decompressing reader by codec.compression.decompress and the
	// web.sitemap gzip path. Without a cap, a small crafted compressed
	// input can inflate to gigabytes in memory (a "decompression bomb").
	// codec.compression.decompress may override this per-call via the
	// `maxBytes` option; the sitemap gzip path has no opts surface and
	// always uses this default.
	DefaultMaxDecompressBytes = 512 << 20 // 512 MB

	// DefaultMaxArchiveBytes caps the total decompressed size written to
	// disk across all members by fs.archive.extract. Without a cap, a
	// small crafted .tar.gz/.zip can be a decompression bomb: each member
	// is copied via io.Copy with no limit on the individual or cumulative
	// output size. Callers may override per-call via the `maxTotalBytes`
	// option.
	DefaultMaxArchiveBytes = 1 << 30 // 1 GB

	// DefaultMaxArchiveEntries caps the number of members fs.archive.extract
	// will process from a single archive. Without a cap, an archive with
	// an enormous entry count can exhaust disk inodes / time even if each
	// entry is individually tiny. Callers may override per-call via the
	// `maxEntries` option.
	DefaultMaxArchiveEntries = 100_000

	// DefaultMaxImagePixels caps the total pixel count (width * height)
	// decodeImage will decode from attacker-controlled bytes. Without a
	// cap, image.Decode allocates a full pixel buffer sized from the
	// file's declared width/height — a crafted header can declare an
	// extreme width x height while the file itself stays a few dozen
	// bytes, triggering a multi-gigabyte allocation ("decode bomb").
	// This is a hard cap: image.decode has no per-call opts surface, so
	// unlike the other caps in this file there is no override knob.
	DefaultMaxImagePixels = 64_000_000 // 64 megapixels
)

// readAllCapped reads all of r, capped at max bytes. It uses
// io.LimitReader(r, max+1) so a stream of exactly max bytes still succeeds
// (reading max+1 is what distinguishes "at the cap" from "over the cap"
// without buffering an unbounded amount first). what names what was being
// read, for the error message.
func readAllCapped(r io.Reader, max int64, what string) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s exceeds maxBytes limit (%d)", what, max)
	}
	return b, nil
}
