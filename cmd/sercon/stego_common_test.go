// cmd/sercon/stego_common_test.go
package main

import (
	"bytes"
	"testing"
)

func TestStegoEncodeDecode_RoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
		isText  bool
		pass    string
	}{
		{"plain-text", []byte("hello"), true, ""},
		{"binary", []byte{0, 1, 2, 255}, false, ""},
		{"encrypted", []byte("secret"), true, "pw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := stegoEncodePayload(tc.payload, tc.isText, tc.pass)
			if err != nil {
				t.Fatal(err)
			}
			// readN returns the first n bytes of the full stream.
			data, isText, err := stegoDecodeStream(func(n int) ([]byte, error) {
				if n > len(stream) {
					t.Fatalf("readN(%d) > stream %d", n, len(stream))
				}
				return stream[:n], nil
			}, tc.pass)
			if err != nil {
				t.Fatal(err)
			}
			if isText != tc.isText || !bytes.Equal(data, tc.payload) {
				t.Fatalf("round-trip: got %q isText=%v", data, isText)
			}
		})
	}
}

func TestStegoDecodeStream_WrongPassword(t *testing.T) {
	stream, _ := stegoEncodePayload([]byte("x"), false, "right")
	_, _, err := stegoDecodeStream(func(n int) ([]byte, error) { return stream[:n], nil }, "wrong")
	if err == nil {
		t.Fatal("wrong password should error")
	}
}

func TestStegoDecodeStream_NoMagic(t *testing.T) {
	junk := bytes.Repeat([]byte{0x41}, 64)
	_, _, err := stegoDecodeStream(func(n int) ([]byte, error) { return junk[:n], nil }, "")
	if err == nil {
		t.Fatal("non-stego bytes should error (no magic)")
	}
}
