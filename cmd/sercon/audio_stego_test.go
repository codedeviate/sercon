// cmd/sercon/audio_stego_test.go
package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dop251/goja"
)

// makeWAV builds a minimal PCM WAV (RIFF/fmt/data) with the given bit depth and
// sample count; samples are a deterministic ramp.
func makeWAV(t *testing.T, bits, samples int) []byte {
	t.Helper()
	return makeWAVFmt(t, 1, bits, samples)
}

// makeWAVFmt is makeWAV with an explicit fmt audioFormat (1 = PCM, 3 = IEEE
// float, …) so tests can exercise the parser's format/bit-depth rejection.
func makeWAVFmt(t *testing.T, audioFormat uint16, bits, samples int) []byte {
	t.Helper()
	bytesPerSample := bits / 8
	dataLen := samples * bytesPerSample
	var b bytes.Buffer
	w := func(v ...byte) { b.Write(v) }
	le16 := func(x uint16) []byte { p := make([]byte, 2); binary.LittleEndian.PutUint16(p, x); return p }
	le32 := func(x uint32) []byte { p := make([]byte, 4); binary.LittleEndian.PutUint32(p, x); return p }
	b.WriteString("RIFF")
	w(le32(uint32(36 + dataLen))...)
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	w(le32(16)...)
	w(le16(audioFormat)...)    // encoding (1 = PCM)
	w(le16(1)...)              // mono
	w(le32(8000)...)           // sample rate
	w(le32(uint32(8000 * bytesPerSample))...)
	w(le16(uint16(bytesPerSample))...)
	w(le16(uint16(bits))...)
	b.WriteString("data")
	w(le32(uint32(dataLen))...)
	for i := 0; i < dataLen; i++ {
		b.WriteByte(byte(i % 251)) // deterministic, non-constant
	}
	return b.Bytes()
}

func TestAudioStego_RoundTrip16(t *testing.T) {
	wav := makeWAV(t, 16, 4096)
	out, err := audioStegoEmbed(wav, []byte("hidden audio msg"), true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(wav) {
		t.Fatalf("output length changed: %d vs %d", len(out), len(wav))
	}
	data, isText, err := audioStegoExtract(out, "")
	if err != nil {
		t.Fatal(err)
	}
	if !isText || string(data) != "hidden audio msg" {
		t.Fatalf("got %q isText=%v", data, isText)
	}
}

func TestAudioStego_HighBytesUntouched16(t *testing.T) {
	wav := makeWAV(t, 16, 4096)
	out, _ := audioStegoEmbed(wav, []byte("x"), false, "pw")
	info, _ := parseWAV(wav)
	// 16-bit LE: high byte is at odd offsets within data — must be unchanged.
	for i := info.dataStart + 1; i < info.dataStart+info.dataLen; i += 2 {
		if out[i] != wav[i] {
			t.Fatalf("high byte at %d changed", i)
		}
	}
}

func TestAudioStego_8bitAndCapacity(t *testing.T) {
	wav := makeWAV(t, 8, 2048)
	cap, err := audioCapacity(wav)
	if err != nil {
		t.Fatal(err)
	}
	if cap != 2048/8-stegoHeaderLen {
		t.Fatalf("capacity = %d", cap)
	}
	out, err := audioStegoEmbed(wav, []byte("8bit"), true, "")
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := audioStegoExtract(out, "")
	if err != nil || string(data) != "8bit" {
		t.Fatalf("8-bit round-trip failed: %q %v", data, err)
	}
}

func TestAudioStego_Errors(t *testing.T) {
	if _, err := parseWAV([]byte("not a wav")); err == nil {
		t.Fatal("non-RIFF should error")
	}
	wav := makeWAV(t, 16, 64)
	if _, err := audioStegoEmbed(wav, bytes.Repeat([]byte{1}, 10000), false, ""); err == nil {
		t.Fatal("payload too large should error")
	}
	if _, _, err := audioStegoExtract(makeWAV(t, 16, 4096), ""); err == nil {
		t.Fatal("clean WAV extract should error (no magic)")
	}
}

func TestAudioStego_UnsupportedFormats(t *testing.T) {
	// IEEE float (audioFormat=3) is a valid RIFF/fmt/data WAV but not PCM.
	if _, err := parseWAV(makeWAVFmt(t, 3, 16, 4096)); err == nil {
		t.Fatal("non-PCM encoding (audioFormat=3) should error")
	}
	// 24-bit PCM is valid PCM but an unsupported bit depth (only 8/16-bit).
	if _, err := parseWAV(makeWAVFmt(t, 1, 24, 4096)); err == nil {
		t.Fatal("24-bit PCM should error")
	}
}

func TestAudioStegoGoja_RoundTrip(t *testing.T) {
	wav := makeWAV(t, 16, 4096)
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range audioStegoNamespace(vm) {
		_ = obj.Set(k, v)
	}
	ans := vm.NewObject()
	_ = ans.Set("stego", obj)
	_ = vm.Set("audio", ans)
	if err := vm.Set("wav", wav); err != nil {
		t.Fatal(err)
	}
	v, err := vm.RunString(`
		const out = audio.stego.embed(wav, "hi", { password: "pw" });
		const cap = audio.stego.capacity(wav).bytes;
		audio.stego.extract(out.bytes, { password: "pw" }) + "|" + (cap > 0);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "hi|true" {
		t.Fatalf("got %q", v.String())
	}
}
