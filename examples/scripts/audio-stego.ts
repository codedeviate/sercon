// audio.stego — hide a secret in a WAV file's PCM samples (LSB).
// Synthesizes a tiny 16-bit PCM WAV in-script, embeds, and recovers.
const SAMPLES = 4096;
const dataLen = SAMPLES * 2;
const buf = new Uint8Array(44 + dataLen);
const dv = new DataView(buf.buffer);
const ascii = (off: number, s: string) => { for (let i = 0; i < s.length; i++) buf[off + i] = s.charCodeAt(i); };
ascii(0, "RIFF"); dv.setUint32(4, 36 + dataLen, true); ascii(8, "WAVE");
ascii(12, "fmt "); dv.setUint32(16, 16, true);
dv.setUint16(20, 1, true);   // PCM
dv.setUint16(22, 1, true);   // mono
dv.setUint32(24, 8000, true);
dv.setUint32(28, 8000 * 2, true);
dv.setUint16(32, 2, true);   // block align
dv.setUint16(34, 16, true);  // bits/sample
ascii(36, "data"); dv.setUint32(40, dataLen, true);
for (let i = 0; i < SAMPLES; i++) dv.setInt16(44 + i * 2, (i % 1000) - 500, true); // a quiet ramp

const cap = audio.stego.capacity(buf).bytes;
runtime.assert.ok(cap > 0, "wav has capacity");
const out = audio.stego.embed(buf, "audio secret", { password: "pw" });
const msg = audio.stego.extract(out.bytes, { password: "pw" });
runtime.assert.equal(msg, "audio secret", "wav secret recovered");
runtime.log("audio-stego OK:", cap, "bytes capacity; recovered:", msg);
