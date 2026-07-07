package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/htmlindex"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// charsetNamespace builds the `text.*` member map. The three members all
// share the same JS surface shape — binary in, structured out for detect,
// string out for decode, bytes out for encode. Charset names follow the
// HTML5 / WHATWG aliases that `htmlindex.Get` understands (UTF-8,
// ISO-8859-1, Windows-1252, Shift_JIS, GBK, …).
func charsetNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"detect": scriptengine.PromisifyAsyncLegacy(vm, loop, charsetDetect),
		"decode": scriptengine.PromisifyAsyncLegacy(vm, loop, charsetDecode),
		"encode": scriptengine.PromisifyAsyncLegacy(vm, loop, charsetEncode),
	}
}

// charsetDetect runs the saintfish/chardet detector over the input bytes and
// returns the most-confident match plus the full candidate list. chardet
// reports an integer confidence on a 0–100 scale; we surface it verbatim
// so scripts can compare against the publisher's docs.
func charsetDetect(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	in, err := exportBytes(call.Argument(0))
	if err != nil {
		return nil, fmt.Errorf("text.detect: %w", err)
	}
	if len(in) == 0 {
		return nil, errors.New("text.detect: empty input")
	}
	results, err := chardet.NewTextDetector().DetectAll(in)
	if err != nil || len(results) == 0 {
		if err == nil {
			err = errors.New("no charset candidates")
		}
		return nil, fmt.Errorf("text.detect: %w", err)
	}
	top := results[0]
	out := map[string]any{
		"charset":    top.Charset,
		"confidence": top.Confidence,
	}
	if top.Language != "" {
		out["language"] = top.Language
	}
	candidates := make([]map[string]any, 0, len(results))
	for _, r := range results {
		c := map[string]any{
			"charset":    r.Charset,
			"confidence": r.Confidence,
		}
		if r.Language != "" {
			c["language"] = r.Language
		}
		candidates = append(candidates, c)
	}
	out["candidates"] = candidates
	return out, nil
}

// charsetDecode converts a byte sequence in `charset` to a JS string (which
// goja stores as Go UTF-8). htmlindex.Get accepts every name the WHATWG
// Encoding Living Standard catalogues (`UTF-8`, `ISO-8859-1`,
// `Windows-1252`, `Shift_JIS`, `GBK`, etc., plus all their documented
// aliases) so callers don't have to memorise a sercon-specific list.
func charsetDecode(_ context.Context, call goja.FunctionCall) (string, error) {
	in, err := exportBytes(call.Argument(0))
	if err != nil {
		return "", fmt.Errorf("text.decode: %w", err)
	}
	charset := call.Argument(1).String()
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return "", fmt.Errorf("text.decode: unknown charset %q", charset)
	}
	decoded, err := enc.NewDecoder().Bytes(in)
	if err != nil {
		return "", fmt.Errorf("text.decode: %w", err)
	}
	return string(decoded), nil
}

// charsetEncode converts a UTF-8 string to a byte sequence in `charset`.
// Characters with no representation in the target encoding produce an
// error from the encoder rather than being silently dropped — callers
// who want lossy behaviour can pre-process the input themselves.
func charsetEncode(_ context.Context, call goja.FunctionCall) ([]byte, error) {
	text := call.Argument(0).String()
	charset := call.Argument(1).String()
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return nil, fmt.Errorf("text.encode: unknown charset %q", charset)
	}
	out, err := enc.NewEncoder().Bytes([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("text.encode: %w", err)
	}
	return out, nil
}
