// audio formats — synthesize a WAV, probe it, convert to FLAC, probe the result.
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

// Build a 0.25s 8kHz mono 16-bit PCM WAV from scratch (44-byte header + samples).
const RATE = 8000, FRAMES = 2000, dataLen = FRAMES * 2;
const buf = new Uint8Array(44 + dataLen);
const dv = new DataView(buf.buffer);
const ascii = (off: number, s: string) => { for (let i = 0; i < s.length; i++) buf[off + i] = s.charCodeAt(i); };
ascii(0, "RIFF"); dv.setUint32(4, 36 + dataLen, true); ascii(8, "WAVE");
ascii(12, "fmt "); dv.setUint32(16, 16, true);
dv.setUint16(20, 1, true); dv.setUint16(22, 1, true);
dv.setUint32(24, RATE, true); dv.setUint32(28, RATE * 2, true);
dv.setUint16(32, 2, true); dv.setUint16(34, 16, true);
ascii(36, "data"); dv.setUint32(40, dataLen, true);
for (let i = 0; i < FRAMES; i++) dv.setInt16(44 + i * 2, Math.round(8000 * Math.sin(i * 0.1)), true);

const info = audio.info(buf);
runtime.assert.equal(info.format, "wav", "sniffed wav");
runtime.assert.equal(info.sampleRate, RATE, "sample rate");

const flac = audio.convert(buf, { format: "flac" });
const finfo = audio.info(flac.bytes);
runtime.assert.equal(finfo.format, "flac", "converted to flac");
runtime.assert.equal(finfo.channels, info.channels, "channels preserved");

const flacPath = `${tmp}/sercon-audio.flac`;
await fs.writeBytes(flacPath, flac.bytes);
runtime.log("audio-format OK:", info.durationMs + "ms wav ->", flac.bytes.length, "byte flac at", flacPath);
