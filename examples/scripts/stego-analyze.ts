// image.stego detection/analysis — embed a secret, then detect/analyze it and
// render a bit-plane. Writes one PNG to $TMPDIR (printed below).
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

// A real 16x16 PNG carrier (RGB), 89 bytes.
const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);

const stego = image.stego.embed(PNG, "find me");
const det = image.stego.detect(stego.bytes);
runtime.assert.ok(det.sercon, "detect finds the sercon payload");
runtime.assert.ok(det.suspicious, "detect flags it suspicious");

const report = image.stego.analyze(stego.bytes);
runtime.assert.equal(report.channels.length, 3, "three channels analyzed");
runtime.assert.ok(report.verdict.suspicious, "analyze verdict suspicious");

const planePath = `${tmp}/sercon-bitplane.png`;
const bp = image.stego.bitplane(stego.bytes, { channel: "rgb", plane: 0 });
await fs.writeBytes(planePath, bp.bytes);
runtime.log("stego-analyze OK:", report.verdict.reasons.join("; "), "— bit-plane:", planePath);
