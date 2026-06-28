// cmd/sercon/audio_format_goja_test.go
package main

import (
	"testing"

	"github.com/dop251/goja"
)

func audioVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range audioFormatMembers(vm) {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("audio", obj)
	wav, err := encodeWAV(makePCM(8000, 1, 2048))
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Set("wav", wav); err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestAudioGoja_InfoConvert(t *testing.T) {
	vm := audioVM(t)
	v, err := vm.RunString(`
		const info = audio.info(wav);
		const flac = audio.convert(wav, { format: "flac" });
		const finfo = audio.info(flac.bytes);
		info.format + "|" + info.sampleRate + "|" + info.channels + "|" + finfo.format + "|" + (finfo.channels === info.channels);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "wav|8000|1|flac|true" {
		t.Fatalf("got %q (want wav|8000|1|flac|true)", got)
	}
}

func TestAudioGoja_DecodeEncode(t *testing.T) {
	vm := audioVM(t)
	v, err := vm.RunString(`
		const d = audio.decode(wav);
		const re = audio.encode(d.pcm, { format: "wav", sampleRate: d.sampleRate, channels: d.channels });
		const back = audio.info(re.bytes);
		(d.pcm.length > 0) + "|" + back.sampleRate + "|" + back.channels;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "true|8000|1" {
		t.Fatalf("got %q", got)
	}
}

func TestAudioGoja_EncodeLossyRejected(t *testing.T) {
	vm := audioVM(t)
	if _, err := vm.RunString(`audio.convert(wav, { format: "mp3" })`); err == nil {
		t.Fatal("convert to mp3 should throw (no pure-Go encoder)")
	}
}

func TestAudioGoja_EncodeMissingOpts(t *testing.T) {
	cases := map[string]string{
		"no opts":                     `audio.encode(audio.decode(wav).pcm)`,
		"missing sampleRate/channels": `audio.encode(audio.decode(wav).pcm, { format: "wav" })`,
		"lossy format":                `audio.encode(audio.decode(wav).pcm, { format: "mp3", sampleRate: 8000, channels: 1 })`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			vm := audioVM(t)
			if _, err := vm.RunString(src); err == nil {
				t.Fatalf("%s: expected a thrown error, got nil", name)
			}
		})
	}
}
