// cmd/sercon/audio_format_test.go
package main

import (
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
