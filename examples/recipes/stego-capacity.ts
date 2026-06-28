// Recipe: multi-bit LSB raises capacity. A payload too large for the default
// 1-bit depth fits at 4 bits/channel; extract auto-detects the depth from the
// header. Writes the 4-bit stego PNG to $TMPDIR.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const carrier = await fs.readBytes(data("small.png"));
const cap1 = image.stego.capacity(carrier, { bits: 1 }).bytes;
const cap4 = image.stego.capacity(carrier, { bits: 4 }).bytes;
runtime.assert.ok(cap4 > cap1, "higher bit depth means more capacity");

// A payload that overflows 1-bit capacity but fits comfortably at 4-bit.
const payload = "A".repeat(cap1 + Math.floor((cap4 - cap1) / 2));
runtime.assert.ok(payload.length > cap1 && payload.length <= cap4, "payload only fits at higher depth");

// It must be rejected at 1 bit...
let tooBig = false;
try { image.stego.embed(carrier, payload, { bits: 1 }); } catch { tooBig = true; }
runtime.assert.ok(tooBig, "payload too large for 1-bit embedding");

// ...and succeed at 4 bits. Extract needs no depth argument.
const stego = image.stego.embed(carrier, payload, { bits: 4 });
const outPath = `${tmp}/sercon-stego-4bit.png`;
await fs.writeBytes(outPath, stego.bytes);
const back = image.stego.extract(await fs.readBytes(outPath));
runtime.assert.equal(back, payload, "4-bit round-trip (depth auto-detected)");

runtime.log(`stego-capacity: 1-bit holds ${cap1}B, 4-bit holds ${cap4}B — hid ${payload.length}B at 4-bit into`, outPath);
