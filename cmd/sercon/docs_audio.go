// cmd/sercon/docs_audio.go
package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// audioDocs documents the `audio` global (format read/write + stego).
func audioDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"info": {
			Summary:    "Probe an audio file's metadata without returning samples. Detects the container by magic bytes (WAV/FLAC/MP3/OGG/AIFF) and reports sample rate, channels, source bit depth, frame count, and duration.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "Audio file path or raw bytes."},
			},
			ReturnType: "{ format: string; sampleRate: number; channels: number; bitDepth: number; frames: number; durationMs: number }",
			Returns:    "Metadata describing the source audio.",
			Errors:     "Throws if the source is not a recognized/decodable audio format.",
			Example:    `const meta = audio.info("song.flac"); // { format: "flac", sampleRate: 44100, ... }`,
		},
		"decode": {
			Summary:    "Decode an audio file (WAV/FLAC/MP3/OGG/AIFF) to canonical 16-bit LE interleaved PCM bytes plus metadata. Samples are normalized to 16-bit regardless of source bit depth.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "Audio file path or raw bytes."},
			},
			ReturnType: "{ format: string; sampleRate: number; channels: number; bitDepth: number; frames: number; durationMs: number; pcm: Uint8Array }",
			Returns:    "Metadata plus a pcm field containing raw 16-bit LE interleaved samples as a Uint8Array.",
			Errors:     "Throws if the source is not a recognized/decodable audio format.",
			Example:    `const d = audio.decode("song.wav"); // d.pcm is a Uint8Array of 16-bit LE samples`,
		},
		"encode": {
			Summary:    "Encode raw 16-bit LE interleaved PCM bytes into a lossless container (WAV, FLAC, or AIFF). Optionally write to a file via opts.dest.",
			Params: []scriptengine.Param{
				{Name: "pcm", Type: "Uint8Array", Desc: "Raw 16-bit LE interleaved samples (as returned by audio.decode)."},
				{Name: "opts", Type: `{ format: "wav" | "flac" | "aiff"; sampleRate: number; channels: number; dest?: string }`, Desc: "format, sampleRate, and channels are required. dest writes to a file path instead of returning bytes."},
			},
			ReturnType: "{ bytes: Uint8Array } | { path: string }",
			Returns:    "The encoded audio bytes ({ bytes }), or { path } when opts.dest was given.",
			Errors:     "Throws for MP3/OGG (no pure-Go encoder), if sampleRate/channels are missing or <=0, or if the PCM length is not a whole number of frames.",
			Example:    `const out = audio.encode(d.pcm, { format: "flac", sampleRate: d.sampleRate, channels: d.channels });`,
		},
		"convert": {
			Summary:    "Decode any supported audio source (WAV/FLAC/MP3/OGG/AIFF) and re-encode it as a lossless container (WAV, FLAC, or AIFF). Optionally write to a file via opts.dest.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "Audio file path or raw bytes of any supported format."},
				{Name: "opts", Type: `{ format: "wav" | "flac" | "aiff"; dest?: string }`, Desc: "format is required. dest writes to a file path instead of returning bytes."},
			},
			ReturnType: "{ bytes: Uint8Array } | { path: string }",
			Returns:    "The re-encoded audio bytes ({ bytes }), or { path } when opts.dest was given.",
			Errors:     "Throws if opts.format is missing, if the source is unrecognized, or if the target format is MP3/OGG (no pure-Go encoder).",
			Example:    `const flac = audio.convert("song.wav", { format: "flac" });`,
		},
		"stego.embed": {
			Summary:    "Hide a payload in a WAV audio file using least-significant-bit steganography on the PCM samples (8- or 16-bit; one bit per sample's low byte — the audio is essentially unchanged). A non-empty password encrypts the payload (AES-256-GCM). Returns the modified WAV bytes. One to four bits are stored per PCM sample (the `bits` option, default 1); higher values raise capacity but make the change more audible and easier to detect.",
			Params: []scriptengine.Param{
				{Name: "cover", Type: "string | Uint8Array", Desc: "The carrier WAV: a file path or raw WAV bytes (PCM, 8- or 16-bit)."},
				{Name: "payload", Type: "string | Uint8Array", Desc: "Data to hide. A string is stored as UTF-8 text; a Uint8Array as binary."},
				{Name: "opts", Type: "{ password?: string; dest?: string; bits?: number }", Optional: true, Desc: "password encrypts the payload; dest writes the resulting WAV to that path instead of returning bytes. bits: payload bits per sample, an integer 1..4 (default 1)."},
			},
			ReturnType: "{ bytes: Uint8Array } | { path: string }",
			Returns:    "The modified WAV bytes ({ bytes }), or { path } when opts.dest was given.",
			Errors:     "Throws if the cover is not a PCM WAV, is an unsupported bit depth (only 8/16-bit), if the payload exceeds capacity, if the payload is not a string/Uint8Array, or if writing dest fails.",
			Example:    `const out = audio.stego.embed("song.wav", "secret", { password: "p" });`,
		},
		"stego.extract": {
			Summary:    "Recover a payload hidden by audio.stego.embed. Reads the PCM sample LSBs, verifies the sercon header, and returns the payload as a string (if embedded as text) or a Uint8Array. The bit depth is read from the header, so no `bits` argument is needed.",
			Params: []scriptengine.Param{
				{Name: "cover", Type: "string | Uint8Array", Desc: "The stego WAV: a file path or raw bytes."},
				{Name: "opts", Type: "{ password?: string }", Optional: true, Desc: "The password used at embed time, required when the payload was encrypted."},
			},
			ReturnType: "string | Uint8Array",
			Returns:    "The recovered payload — a string when embedded as text, otherwise a Uint8Array.",
			Errors:     "Throws if the cover is not a PCM WAV, if no sercon payload is present, if encrypted without a password, or on decryption failure.",
			Example:    `const msg = audio.stego.extract("song.wav", { password: "p" });`,
		},
		"stego.capacity": {
			Summary:    "Report the maximum payload size in bytes a WAV carrier can hold via LSB steganography — one bit per PCM sample, minus the fixed header. Encryption adds ~44 bytes of overhead.",
			Params: []scriptengine.Param{
				{Name: "cover", Type: "string | Uint8Array", Desc: "The carrier WAV: a file path or raw bytes."},
				{Name: "opts", Type: "{ bits?: number }", Optional: true, Desc: "bits: report capacity at this depth (integer 1..4, default 1)."},
			},
			ReturnType: "{ bytes: number; bits: number }",
			Returns:    "An object: bytes is the maximum plaintext payload size at the requested depth; bits echoes that depth.",
			Errors:     "Throws if the cover is not a supported PCM WAV, or a TypeError if bits is not an integer 1..4.",
			Example:    `const room = audio.stego.capacity("song.wav", { bits: 4 }).bytes;`,
		},
	}
}
