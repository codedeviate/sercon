// cmd/sercon/audio_format.go
package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-audio/aiff"
	goaudio "github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// pcmAudio is the canonical decoded representation: 16-bit signed, interleaved,
// little-endian samples plus source metadata.
type pcmAudio struct {
	sampleRate int
	channels   int
	bitDepth   int // source bit depth (reported); samples are 16-bit
	samples    []int16
}

func (p pcmAudio) frames() int {
	if p.channels == 0 {
		return 0
	}
	return len(p.samples) / p.channels
}

// down16 normalizes a source sample of srcBits depth to a 16-bit signed value.
func down16(v, srcBits int) int16 {
	switch {
	case srcBits == 16:
		return int16(v)
	case srcBits > 16:
		return int16(v >> (srcBits - 16))
	case srcBits == 8:
		// go-audio yields 8-bit PCM as signed-centered ints already in -128..127
		// for AIFF and 0..255 for unsigned WAV; verify against the lib and adjust
		// if a constant 128 offset appears (the round-trip test is the oracle).
		return int16(v << 8)
	default:
		return int16(v << (16 - srcBits))
	}
}

// sniffAudioFormat detects the container by leading magic bytes.
func sniffAudioFormat(data []byte) string {
	n := len(data)
	if n < 4 {
		return ""
	}
	switch {
	case n >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return "wav"
	case string(data[0:4]) == "fLaC":
		return "flac"
	case string(data[0:4]) == "OggS":
		return "ogg"
	case n >= 12 && string(data[0:4]) == "FORM" && (string(data[8:12]) == "AIFF" || string(data[8:12]) == "AIFC"):
		return "aiff"
	case n >= 3 && string(data[0:3]) == "ID3":
		return "mp3"
	case n >= 2 && data[0] == 0xFF && data[1]&0xE0 == 0xE0:
		return "mp3" // MPEG audio frame sync (probabilistic — any 0xFF 0xEx-or-higher byte pair matches)
	default:
		return ""
	}
}

// pcmFromIntBuffer converts a go-audio IntBuffer to canonical 16-bit PCM.
func pcmFromIntBuffer(buf *goaudio.IntBuffer) pcmAudio {
	src := buf.SourceBitDepth
	if src == 0 {
		src = 16
	}
	out := make([]int16, len(buf.Data))
	for i, v := range buf.Data {
		out[i] = down16(v, src)
	}
	return pcmAudio{
		sampleRate: buf.Format.SampleRate,
		channels:   buf.Format.NumChannels,
		bitDepth:   src,
		samples:    out,
	}
}

// intBufferFromPCM builds a 16-bit go-audio IntBuffer from canonical PCM.
func intBufferFromPCM(p pcmAudio) *goaudio.IntBuffer {
	data := make([]int, len(p.samples))
	for i, s := range p.samples {
		data[i] = int(s)
	}
	return &goaudio.IntBuffer{
		Format:         &goaudio.Format{NumChannels: p.channels, SampleRate: p.sampleRate},
		Data:           data,
		SourceBitDepth: 16,
	}
}

// memWriteSeeker is an in-memory io.WriteSeeker (go-audio encoders backpatch
// container sizes, so a plain bytes.Buffer won't do).
type memWriteSeeker struct {
	buf []byte
	pos int64
}

func (m *memWriteSeeker) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		grown := make([]byte, end)
		copy(grown, m.buf)
		m.buf = grown
	}
	copy(m.buf[m.pos:end], p)
	m.pos = end
	return len(p), nil
}

func (m *memWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = m.pos + offset
	case io.SeekEnd:
		abs = int64(len(m.buf)) + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}
	if abs < 0 {
		return 0, fmt.Errorf("negative position")
	}
	m.pos = abs
	return abs, nil
}

func decodeWAV(data []byte) (pcmAudio, error) {
	d := wav.NewDecoder(bytes.NewReader(data))
	if !d.IsValidFile() {
		return pcmAudio{}, fmt.Errorf("invalid WAV")
	}
	buf, err := d.FullPCMBuffer()
	if err != nil {
		return pcmAudio{}, err
	}
	return pcmFromIntBuffer(buf), nil
}

func encodeWAV(p pcmAudio) ([]byte, error) {
	ws := &memWriteSeeker{}
	enc := wav.NewEncoder(ws, p.sampleRate, 16, p.channels, 1) // audioFormat 1 = PCM
	if err := enc.Write(intBufferFromPCM(p)); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return ws.buf, nil
}

func decodeAIFF(data []byte) (pcmAudio, error) {
	d := aiff.NewDecoder(bytes.NewReader(data))
	if !d.IsValidFile() {
		return pcmAudio{}, fmt.Errorf("invalid AIFF")
	}
	buf, err := d.FullPCMBuffer()
	if err != nil {
		return pcmAudio{}, err
	}
	return pcmFromIntBuffer(buf), nil
}

func encodeAIFF(p pcmAudio) ([]byte, error) {
	ws := &memWriteSeeker{}
	enc := aiff.NewEncoder(ws, p.sampleRate, 16, p.channels)
	if err := enc.Write(intBufferFromPCM(p)); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return ws.buf, nil
}

// audioInfo returns metadata for any supported source (decodes as needed).
// NOTE: Task 2 will replace this body with a call to decodeAudio (full dispatch).
// For Task 1, only WAV/AIFF are supported.
func audioInfo(data []byte) (pcmAudio, error) {
	switch sniffAudioFormat(data) {
	case "wav":
		return decodeWAV(data)
	case "aiff":
		return decodeAIFF(data)
	default:
		return pcmAudio{}, fmt.Errorf("audio: unsupported format")
	}
}
