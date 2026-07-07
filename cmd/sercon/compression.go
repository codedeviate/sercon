package main

import (
	"bytes"
	"compress/bzip2"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	dsnetbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// compressionAlgos enumerates the algorithms the binding accepts. Kept as a
// package-level slice so the namespace's `algos()` helper and the d.ts
// emitter (via the `compress` / `decompress` signatures) stay in sync.
var compressionAlgos = []string{
	"gzip", "deflate", "zlib", "bzip2", "zstd", "brotli", "lz4", "xz", "snappy",
}

// compressionNamespace builds the `codec.compression.*` member map. Inputs are
// either strings (interpreted as UTF-8 bytes) or Uint8Array / ArrayBuffer
// values exported from JS as `[]byte` by goja. Outputs are `[]byte`, which
// goja surfaces back to JS as an ArrayBuffer; scripts typically wrap that
// with `new Uint8Array(buf)` to iterate.
func compressionNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	algos := make([]string, len(compressionAlgos))
	copy(algos, compressionAlgos)
	return map[string]any{
		"compress":   scriptengine.PromisifyAsyncLegacy(vm, loop, compressCall),
		"decompress": scriptengine.PromisifyAsyncLegacy(vm, loop, decompressCall),
		"algos":      func() []string { return algos },
	}
}

// exportBytes pulls a binary input out of a JS argument. Strings become
// their UTF-8 byte sequence; Uint8Array and ArrayBuffer round-trip through
// goja as `[]byte` and `goja.ArrayBuffer` respectively.
func exportBytes(v goja.Value) ([]byte, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, errors.New("input is undefined or null")
	}
	switch e := v.Export().(type) {
	case string:
		return []byte(e), nil
	case []byte:
		return e, nil
	case goja.ArrayBuffer:
		return e.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported input type %T (want string, Uint8Array, or ArrayBuffer)", e)
	}
}

func compressCall(_ context.Context, call goja.FunctionCall) ([]byte, error) {
	algo := strings.ToLower(call.Argument(0).String())
	in, err := exportBytes(call.Argument(1))
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	return compressBytes(algo, in)
}

func decompressCall(_ context.Context, call goja.FunctionCall) ([]byte, error) {
	algo := strings.ToLower(call.Argument(0).String())
	in, err := exportBytes(call.Argument(1))
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	opts := thirdArgAsMap(call)
	maxBytes := int64(optInt(opts, "maxBytes", DefaultMaxDecompressBytes))
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDecompressBytes
	}
	return decompressBytes(algo, in, maxBytes)
}

// compressBytes routes by algorithm name. Each branch handles writer
// creation, write, and Close — Close is what flushes trailers for most
// compressors, so missing it would silently truncate the output.
func compressBytes(algo string, in []byte) ([]byte, error) {
	switch algo {
	case "snappy":
		// snappy has a one-shot encoder; no framing.
		return snappy.Encode(nil, in), nil
	case "gzip":
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "deflate":
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "zlib":
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "bzip2":
		// stdlib's compress/bzip2 is decompression-only; dsnet/compress/bzip2
		// provides a pure-Go encoder.
		var buf bytes.Buffer
		w, err := dsnetbzip2.NewWriter(&buf, &dsnetbzip2.WriterConfig{
			Level: dsnetbzip2.DefaultCompression,
		})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "zstd":
		var buf bytes.Buffer
		w, err := zstd.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "brotli":
		var buf bytes.Buffer
		w := brotli.NewWriter(&buf)
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "lz4":
		var buf bytes.Buffer
		w := lz4.NewWriter(&buf)
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "xz":
		var buf bytes.Buffer
		w, err := xz.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(in); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q (supported: %s)", algo,
			strings.Join(compressionAlgos, ", "))
	}
}

// decompressBytes routes by algorithm name. Readers that implement
// `io.Closer` are closed via a deferred call inside their case so we don't
// leak any compressor-side resources (decoder pools, internal buffers).
//
// maxBytes caps the decompressed output size, guarding against a
// decompression bomb (a small crafted input that inflates to gigabytes). A
// non-positive maxBytes means "use the default". Every reader-based
// algorithm goes through readAllCapped (io.LimitReader(zr, maxBytes+1) plus
// an overflow check); snappy has no streaming reader, so it's guarded via
// the format's own length header (snappy.DecodedLen) before ever
// allocating the output buffer.
func decompressBytes(algo string, in []byte, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDecompressBytes
	}
	switch algo {
	case "snappy":
		n, err := snappy.DecodedLen(in)
		if err != nil {
			return nil, err
		}
		if int64(n) > maxBytes {
			return nil, fmt.Errorf("decompressed output exceeds maxBytes limit (%d)", maxBytes)
		}
		return snappy.Decode(nil, in)
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(in))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return readAllCapped(zr, maxBytes, "decompressed output")
	case "deflate":
		zr := flate.NewReader(bytes.NewReader(in))
		defer func() { _ = zr.Close() }()
		return readAllCapped(zr, maxBytes, "decompressed output")
	case "zlib":
		zr, err := zlib.NewReader(bytes.NewReader(in))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return readAllCapped(zr, maxBytes, "decompressed output")
	case "bzip2":
		// stdlib bzip2 reader is fine for decompression.
		return readAllCapped(bzip2.NewReader(bytes.NewReader(in)), maxBytes, "decompressed output")
	case "zstd":
		zr, err := zstd.NewReader(bytes.NewReader(in))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return readAllCapped(zr, maxBytes, "decompressed output")
	case "brotli":
		return readAllCapped(brotli.NewReader(bytes.NewReader(in)), maxBytes, "decompressed output")
	case "lz4":
		return readAllCapped(lz4.NewReader(bytes.NewReader(in)), maxBytes, "decompressed output")
	case "xz":
		zr, err := xz.NewReader(bytes.NewReader(in))
		if err != nil {
			return nil, err
		}
		return readAllCapped(zr, maxBytes, "decompressed output")
	default:
		return nil, fmt.Errorf("unknown algorithm %q (supported: %s)", algo,
			strings.Join(compressionAlgos, ", "))
	}
}
