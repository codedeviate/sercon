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
			stream, err := stegoEncodePayload(tc.payload, tc.isText, tc.pass, 1)
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
	stream, _ := stegoEncodePayload([]byte("x"), false, "right", 1)
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

func TestLSBCarrier_MultiBitRoundTrip(t *testing.T) {
	payload := []byte("multi-bit LSB payload spanning many carrier units 0123456789")
	for n := 1; n <= 4; n++ {
		header := marshalStegoHeader(byte(n-1)<<flagBitsShift, uint32(len(payload)))
		pix := make([]byte, 8192)
		for i := range pix {
			pix[i] = byte(i) // non-zero carrier so we exercise clearing
		}
		c := lsbCarrier{pix: pix, count: len(pix), at: func(k int) int { return k }}
		if err := lsbWriteStream(c, header, payload, n); err != nil {
			t.Fatalf("n=%d write: %v", n, err)
		}
		got, err := lsbReadStream(c, stegoHeaderLen+len(payload))
		if err != nil {
			t.Fatalf("n=%d read: %v", n, err)
		}
		if !bytes.Equal(got[:stegoHeaderLen], header) {
			t.Fatalf("n=%d header mismatch", n)
		}
		if !bytes.Equal(got[stegoHeaderLen:], payload) {
			t.Fatalf("n=%d payload mismatch: %q", n, got[stegoHeaderLen:])
		}
	}
}

func TestStegoEncodePayload_PacksBits(t *testing.T) {
	for n := 1; n <= 4; n++ {
		stream, err := stegoEncodePayload([]byte("x"), false, "", n)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		flags, _, err := parseStegoHeader(stream[:stegoHeaderLen])
		if err != nil {
			t.Fatal(err)
		}
		if got := int((flags&flagBitsMask)>>flagBitsShift) + 1; got != n {
			t.Fatalf("n=%d packed/unpacked as %d", n, got)
		}
	}
	// Legacy: a header with the bit-depth field zero decodes as N=1.
	legacy := marshalStegoHeader(0, 0)
	flags, _, _ := parseStegoHeader(legacy)
	if got := int((flags&flagBitsMask)>>flagBitsShift) + 1; got != 1 {
		t.Fatalf("legacy zero must decode as N=1, got %d", got)
	}
}

func TestLSBCapacityBytes(t *testing.T) {
	cases := []struct {
		count, n, want int
	}{
		{80, 4, 0},  // header-only → no payload room
		{40, 2, 0},  // below header → clamps to 0, never negative
		{96, 1, 2},  // 16 spare units × 1 bit = 16 bits = 2 bytes
		{96, 4, 8},  // 16 spare units × 4 bits = 64 bits = 8 bytes
	}
	for _, c := range cases {
		if got := lsbCapacityBytes(c.count, c.n); got != c.want {
			t.Errorf("lsbCapacityBytes(%d,%d) = %d, want %d", c.count, c.n, got, c.want)
		}
	}
}
