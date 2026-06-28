// cmd/sercon/audio_format.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/dop251/goja"
	"github.com/go-audio/aiff"
	goaudio "github.com/go-audio/audio"
	"github.com/go-audio/wav"
	mp3 "github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
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
func audioInfo(data []byte) (pcmAudio, error) {
	return decodeAudio(data)
}

// decodeMP3 decodes MP3 to canonical PCM (go-mp3 always yields 16-bit LE stereo).
func decodeMP3(data []byte) (pcmAudio, error) {
	d, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return pcmAudio{}, err
	}
	raw, err := io.ReadAll(d)
	if err != nil {
		return pcmAudio{}, err
	}
	samples := make([]int16, len(raw)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
	}
	return pcmAudio{sampleRate: d.SampleRate(), channels: 2, bitDepth: 16, samples: samples}, nil
}

// decodeOGG decodes OGG/Vorbis (float32 [-1,1]) to canonical 16-bit PCM.
func decodeOGG(data []byte) (pcmAudio, error) {
	fl, format, err := oggvorbis.ReadAll(bytes.NewReader(data))
	if err != nil {
		return pcmAudio{}, err
	}
	samples := make([]int16, len(fl))
	for i, f := range fl {
		v := f * 32767
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		samples[i] = int16(v)
	}
	return pcmAudio{sampleRate: format.SampleRate, channels: format.Channels, bitDepth: 16, samples: samples}, nil
}

// decodeFLAC decodes FLAC to canonical 16-bit PCM.
func decodeFLAC(data []byte) (pcmAudio, error) {
	stream, err := flac.New(bytes.NewReader(data))
	if err != nil {
		return pcmAudio{}, err
	}
	defer stream.Close()
	ch := int(stream.Info.NChannels)
	bits := int(stream.Info.BitsPerSample)
	var samples []int16
	for {
		fr, ferr := stream.ParseNext()
		if ferr == io.EOF {
			break
		}
		if ferr != nil {
			return pcmAudio{}, ferr
		}
		if len(fr.Subframes) < ch {
			return pcmAudio{}, fmt.Errorf("flac: frame has %d subframes, expected %d", len(fr.Subframes), ch)
		}
		n := fr.Subframes[0].NSamples
		for i := 0; i < n; i++ {
			for c := 0; c < ch; c++ {
				samples = append(samples, down16(int(fr.Subframes[c].Samples[i]), bits))
			}
		}
	}
	return pcmAudio{sampleRate: int(stream.Info.SampleRate), channels: ch, bitDepth: bits, samples: samples}, nil
}

// encodeFLAC encodes canonical 16-bit PCM to FLAC.
//
// FLAC frame construction with mewkiz is intricate: a stream needs a
// meta.StreamInfo and one or more frame.Frame blocks, each with a frame.Header
// (BlockSize, SampleRate, Channels enum, BitsPerSample, Num) and per-channel
// Subframes (SubHeader{Pred: frame.PredVerbatim} + Samples []int32 + NSamples).
// This follows mewkiz/flac's encode example — if WriteFrame errors or the
// round-trip test fails, align the frame.Header/Subframe/SubHeader fields with
// the current github.com/mewkiz/flac encode example; TestFLAC_RoundTrip is the
// oracle.
func encodeFLAC(p pcmAudio) ([]byte, error) {
	if p.channels < 1 || p.channels > 8 {
		return nil, fmt.Errorf("flac: unsupported channel count %d", p.channels)
	}
	info := &meta.StreamInfo{
		BlockSizeMin:  16,
		BlockSizeMax:  65535,
		SampleRate:    uint32(p.sampleRate),
		NChannels:     uint8(p.channels),
		BitsPerSample: 16,
		NSamples:      uint64(p.frames()),
	}
	ws := &memWriteSeeker{}
	enc, err := flac.NewEncoder(ws, info)
	if err != nil {
		return nil, err
	}
	const block = 4096
	ch := p.channels
	chans := frameChannelsFor(ch) // frame.Channels enum for 1..8 channels
	for start := 0; start < p.frames(); start += block {
		n := block
		if start+n > p.frames() {
			n = p.frames() - start
		}
		subs := make([]*frame.Subframe, ch)
		for c := 0; c < ch; c++ {
			s := make([]int32, n)
			for i := 0; i < n; i++ {
				s[i] = int32(p.samples[(start+i)*ch+c])
			}
			subs[c] = &frame.Subframe{
				SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
				Samples:   s,
				NSamples:  n,
			}
		}
		fr := &frame.Frame{
			Header: frame.Header{
				HasFixedBlockSize: true,
				BlockSize:         uint16(n),
				SampleRate:        uint32(p.sampleRate),
				Channels:          chans,
				BitsPerSample:     16,
			},
			Subframes: subs,
		}
		if werr := enc.WriteFrame(fr); werr != nil {
			return nil, werr
		}
	}
	if cerr := enc.Close(); cerr != nil {
		return nil, cerr
	}
	return ws.buf, nil
}

// frameChannelsFor maps a channel count to the mewkiz frame.Channels enum.
func frameChannelsFor(ch int) frame.Channels {
	switch ch {
	case 1:
		return frame.ChannelsMono
	case 2:
		return frame.ChannelsLR
	case 3:
		return frame.ChannelsLRC
	case 4:
		return frame.ChannelsLRLsRs
	case 5:
		return frame.ChannelsLRCLsRs
	case 6:
		return frame.ChannelsLRCLfeLsRs
	case 7:
		return frame.ChannelsLRCLfeCsSlSr
	case 8:
		return frame.ChannelsLRCLfeLsRsSlSr
	default:
		return frame.ChannelsLRCLfeLsRsSlSr
	}
}

// decodeAudio sniffs the container and decodes to canonical PCM.
func decodeAudio(data []byte) (pcmAudio, error) {
	switch sniffAudioFormat(data) {
	case "wav":
		return decodeWAV(data)
	case "aiff":
		return decodeAIFF(data)
	case "flac":
		return decodeFLAC(data)
	case "mp3":
		return decodeMP3(data)
	case "ogg":
		return decodeOGG(data)
	default:
		return pcmAudio{}, fmt.Errorf("audio: unsupported or unrecognized format")
	}
}

// encodeAudio encodes canonical PCM to a lossless container (wav/flac/aiff).
func encodeAudio(p pcmAudio, format string) ([]byte, error) {
	switch format {
	case "wav":
		return encodeWAV(p)
	case "flac":
		return encodeFLAC(p)
	case "aiff":
		return encodeAIFF(p)
	case "mp3", "ogg":
		return nil, fmt.Errorf("no pure-Go encoder for %s; encode to wav, flac, or aiff", format)
	default:
		return nil, fmt.Errorf("unknown audio format %q", format)
	}
}

// pcmToBytes / pcmFromBytes bridge canonical 16-bit samples and raw LE bytes.
func pcmToBytes(s []int16) []byte {
	b := make([]byte, len(s)*2)
	for i, v := range s {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
	}
	return b
}

func pcmFromBytes(b []byte) []int16 {
	s := make([]int16, len(b)/2)
	for i := range s {
		s[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return s
}

func audioFormatMembers(vm *goja.Runtime) map[string]any {
	infoMap := func(p pcmAudio) map[string]any {
		durationMs := 0
		if p.sampleRate > 0 {
			durationMs = int(int64(p.frames()) * 1000 / int64(p.sampleRate))
		}
		return map[string]any{
			"sampleRate": p.sampleRate,
			"channels":   p.channels,
			"bitDepth":   p.bitDepth,
			"frames":     p.frames(),
			"durationMs": durationMs,
		}
	}
	optsFmtDest := func(arg goja.Value) (string, string) {
		format, dest := "", ""
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			o := arg.ToObject(vm)
			if f := o.Get("format"); f != nil && !goja.IsUndefined(f) {
				format = f.String()
			}
			if d := o.Get("dest"); d != nil && !goja.IsUndefined(d) {
				dest = d.String()
			}
		}
		return format, dest
	}
	emit := func(out []byte, dest string) goja.Value {
		if dest != "" {
			if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
				panic(vm.NewGoError(fmt.Errorf("audio: %w", werr)))
			}
			return vm.ToValue(map[string]any{"path": dest})
		}
		return vm.ToValue(map[string]any{"bytes": out})
	}
	return map[string]any{
		"info": func(call goja.FunctionCall) goja.Value {
			src := stegoSrcBytes(vm, call.Argument(0), "audio.info")
			p, err := audioInfo(src)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("audio.info: %w", err)))
			}
			m := infoMap(p)
			m["format"] = sniffAudioFormat(src)
			return vm.ToValue(m)
		},
		"decode": func(call goja.FunctionCall) goja.Value {
			src := stegoSrcBytes(vm, call.Argument(0), "audio.decode")
			p, err := decodeAudio(src)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("audio.decode: %w", err)))
			}
			m := infoMap(p)
			m["format"] = sniffAudioFormat(src)
			m["pcm"] = pcmToBytes(p.samples)
			return vm.ToValue(m)
		},
		"encode": func(call goja.FunctionCall) goja.Value {
			raw, ok := call.Argument(0).Export().([]byte)
			if !ok {
				panic(vm.NewTypeError("audio.encode: pcm must be a Uint8Array of 16-bit LE samples"))
			}
			arg1 := call.Argument(1)
			if goja.IsUndefined(arg1) || goja.IsNull(arg1) {
				panic(vm.NewTypeError("audio.encode: opts { format, sampleRate, channels } is required"))
			}
			o := arg1.ToObject(vm)
			format, dest := "", ""
			if f := o.Get("format"); f != nil && !goja.IsUndefined(f) {
				format = f.String()
			}
			if d := o.Get("dest"); d != nil && !goja.IsUndefined(d) {
				dest = d.String()
			}
			if format == "" {
				panic(vm.NewTypeError("audio.encode: opts.format is required (wav, flac, aiff)"))
			}
			srVal, chVal := o.Get("sampleRate"), o.Get("channels")
			if srVal == nil || goja.IsUndefined(srVal) || chVal == nil || goja.IsUndefined(chVal) {
				panic(vm.NewTypeError("audio.encode: opts.sampleRate and opts.channels (>0) are required"))
			}
			sampleRate, channels := int(srVal.ToInteger()), int(chVal.ToInteger())
			if sampleRate <= 0 || channels <= 0 {
				panic(vm.NewTypeError("audio.encode: opts.sampleRate and opts.channels must be > 0"))
			}
			if len(raw)%(2*channels) != 0 {
				panic(vm.NewGoError(fmt.Errorf("audio.encode: pcm length %d not a whole number of %d-channel frames", len(raw), channels)))
			}
			p := pcmAudio{sampleRate: sampleRate, channels: channels, bitDepth: 16, samples: pcmFromBytes(raw)}
			out, err := encodeAudio(p, format)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("audio.encode: %w", err)))
			}
			return emit(out, dest)
		},
		"convert": func(call goja.FunctionCall) goja.Value {
			src := stegoSrcBytes(vm, call.Argument(0), "audio.convert")
			format, dest := optsFmtDest(call.Argument(1))
			if format == "" {
				panic(vm.NewTypeError("audio.convert: opts.format is required (wav, flac, aiff)"))
			}
			p, err := decodeAudio(src)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("audio.convert: %w", err)))
			}
			out, err := encodeAudio(p, format)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("audio.convert: %w", err)))
			}
			return emit(out, dest)
		},
	}
}
