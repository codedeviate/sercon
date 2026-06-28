// cmd/sercon/docs_audio.go
package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// audioDocs documents the `audio` global (currently the stego carrier).
func audioDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"stego.embed": {
			Summary:    "Hide a payload in a WAV audio file using least-significant-bit steganography on the PCM samples (8- or 16-bit; one bit per sample's low byte — the audio is essentially unchanged). A non-empty password encrypts the payload (AES-256-GCM). Returns the modified WAV bytes.",
			Params: []scriptengine.Param{
				{Name: "cover", Type: "string | Uint8Array", Desc: "The carrier WAV: a file path or raw WAV bytes (PCM, 8- or 16-bit)."},
				{Name: "payload", Type: "string | Uint8Array", Desc: "Data to hide. A string is stored as UTF-8 text; a Uint8Array as binary."},
				{Name: "opts", Type: "{ password?: string; dest?: string }", Optional: true, Desc: "password encrypts the payload; dest writes the resulting WAV to that path instead of returning bytes."},
			},
			ReturnType: "{ bytes: Uint8Array } | { path: string }",
			Returns:    "The modified WAV bytes ({ bytes }), or { path } when opts.dest was given.",
			Errors:     "Throws if the cover is not a PCM WAV, is an unsupported bit depth (only 8/16-bit), if the payload exceeds capacity, if the payload is not a string/Uint8Array, or if writing dest fails.",
			Example:    `const out = audio.stego.embed("song.wav", "secret", { password: "p" });`,
		},
		"stego.extract": {
			Summary:    "Recover a payload hidden by audio.stego.embed. Reads the PCM sample LSBs, verifies the sercon header, and returns the payload as a string (if embedded as text) or a Uint8Array.",
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
			},
			ReturnType: "{ bytes: number }",
			Returns:    "An object whose bytes field is the maximum plaintext payload size.",
			Errors:     "Throws if the cover is not a supported PCM WAV.",
			Example:    `const room = audio.stego.capacity("song.wav").bytes;`,
		},
	}
}
