// cmd/sercon/audio_format_test.go
package main

import (
	"os"
	"testing"
)

// makePCM builds a deterministic 16-bit PCM fixture (sawtooth ramp).
func makePCM(rate, channels, frames int) pcmAudio {
	s := make([]int16, frames*channels)
	for i := range s {
		s[i] = int16((i % 2000) - 1000) // deterministic ramp/sawtooth
	}
	return pcmAudio{sampleRate: rate, channels: channels, bitDepth: 16, samples: s}
}

func TestSniffAudioFormat(t *testing.T) {
	cases := map[string][]byte{
		"wav":  []byte("RIFF\x00\x00\x00\x00WAVEfmt "),
		"flac": []byte("fLaC\x00\x00\x00\x22"),
		"ogg":  []byte("OggS\x00\x02\x00\x00"),
		"aiff": append([]byte("FORM\x00\x00\x00\x00AIFF"), 0),
		"mp3":  []byte("ID3\x04\x00\x00\x00\x00\x00\x00"),
	}
	for want, data := range cases {
		if got := sniffAudioFormat(data); got != want {
			t.Errorf("sniff(%s) = %q, want %q", want, got, want)
		}
	}
	if got := sniffAudioFormat([]byte("not audio")); got != "" {
		t.Errorf("sniff(garbage) = %q, want empty", got)
	}
}

func TestWAV_RoundTrip(t *testing.T) {
	in := makePCM(8000, 1, 4096)
	wavBytes, err := encodeWAV(in)
	if err != nil {
		t.Fatal(err)
	}
	if sniffAudioFormat(wavBytes) != "wav" {
		t.Fatal("encodeWAV output not sniffed as wav")
	}
	out, err := decodeWAV(wavBytes)
	if err != nil {
		t.Fatal(err)
	}
	if out.sampleRate != 8000 || out.channels != 1 || len(out.samples) != len(in.samples) {
		t.Fatalf("meta mismatch: %+v", out)
	}
	for i := range in.samples {
		if out.samples[i] != in.samples[i] {
			t.Fatalf("sample %d: got %d want %d", i, out.samples[i], in.samples[i])
		}
	}
}

func TestAIFF_RoundTrip(t *testing.T) {
	in := makePCM(44100, 2, 2048)
	aiffBytes, err := encodeAIFF(in)
	if err != nil {
		t.Fatal(err)
	}
	if sniffAudioFormat(aiffBytes) != "aiff" {
		t.Fatal("encodeAIFF output not sniffed as aiff")
	}
	out, err := decodeAIFF(aiffBytes)
	if err != nil {
		t.Fatal(err)
	}
	if out.sampleRate != 44100 || out.channels != 2 || len(out.samples) != len(in.samples) {
		t.Fatalf("meta mismatch: %+v", out)
	}
	for i := range in.samples {
		if out.samples[i] != in.samples[i] {
			t.Fatalf("aiff round-trip sample %d: got %d want %d", i, out.samples[i], in.samples[i])
		}
	}
}

func TestAudioInfo_WAV(t *testing.T) {
	wavBytes, _ := encodeWAV(makePCM(22050, 2, 1000))
	info, err := audioInfo(wavBytes)
	if err != nil {
		t.Fatal(err)
	}
	if info.sampleRate != 22050 || info.channels != 2 {
		t.Fatalf("info = %+v", info)
	}
	if info.frames() != 1000 {
		t.Fatalf("frames = %d, want 1000", info.frames())
	}
}

func TestFLAC_RoundTrip(t *testing.T) {
	in := makePCM(44100, 1, 8192)
	flacBytes, err := encodeFLAC(in)
	if err != nil {
		t.Fatal(err)
	}
	if sniffAudioFormat(flacBytes) != "flac" {
		t.Fatal("encodeFLAC output not sniffed as flac")
	}
	out, err := decodeFLAC(flacBytes)
	if err != nil {
		t.Fatal(err)
	}
	if out.sampleRate != 44100 || out.channels != 1 {
		t.Fatalf("meta mismatch: %+v", out)
	}
	if len(out.samples) != len(in.samples) {
		t.Fatalf("sample count %d != %d", len(out.samples), len(in.samples))
	}
	for i := range in.samples {
		if out.samples[i] != in.samples[i] {
			t.Fatalf("flac round-trip sample %d: got %d want %d", i, out.samples[i], in.samples[i])
		}
	}
}

func TestDecodeAudio_Convert_WAVtoFLAC(t *testing.T) {
	wavBytes, _ := encodeWAV(makePCM(8000, 2, 4096))
	p, err := decodeAudio(wavBytes)
	if err != nil {
		t.Fatal(err)
	}
	flacBytes, err := encodeFLAC(p)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeAudio(flacBytes)
	if err != nil {
		t.Fatal(err)
	}
	if back.channels != 2 || len(back.samples) != len(p.samples) {
		t.Fatalf("wav->flac->pcm mismatch: %+v", back)
	}
}

// MP3/OGG decode: skip-gated — no pure-Go encoder to synthesize fixtures.
func TestDecodeMP3_Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.mp3")
	if err != nil {
		t.Skip("no testdata/tiny.mp3 fixture")
	}
	p, err := decodeMP3(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.sampleRate <= 0 || p.channels != 2 || len(p.samples) == 0 {
		t.Fatalf("mp3 decode looks wrong: %+v", p)
	}
}

func TestDecodeOGG_Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.ogg")
	if err != nil {
		t.Skip("no testdata/tiny.ogg fixture")
	}
	p, err := decodeOGG(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.sampleRate <= 0 || p.channels < 1 || len(p.samples) == 0 {
		t.Fatalf("ogg decode looks wrong: %+v", p)
	}
}
