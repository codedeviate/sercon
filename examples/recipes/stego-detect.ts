// Recipe: steganalysis. Tell a clean image from one carrying a hidden payload,
// and read back the embedding's declared depth and encryption flag — without
// the password. Read-only: nothing is written.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")

const carrier = await fs.readBytes(data("medium.png"));

// The untouched image carries no sercon payload.
const clean = image.stego.detect(carrier);
runtime.assert.ok(!clean.sercon, "clean image has no sercon stego header");

// Hide an encrypted secret at 3 bits/channel, then inspect the result.
const stego = image.stego.embed(carrier, "the eagle lands at midnight", { bits: 3, password: "k" });
const det = image.stego.detect(stego.bytes);
runtime.assert.ok(det.sercon, "detect finds the sercon header");
runtime.assert.equal(det.bits, 3, "detect reports the declared bit depth");
runtime.assert.ok(det.encrypted, "detect flags the encrypted payload");

// analyze() adds the full statistical report (estimatedBits is a coarse hint;
// the header-declared det.bits above is authoritative for sercon payloads).
const report = image.stego.analyze(stego.bytes);

runtime.log(
  `stego-detect: clean.sercon=${clean.sercon} clean.suspicious=${clean.suspicious}`,
  `| stego bits=${det.bits} encrypted=${det.encrypted}`,
  `| analyze verdict=${report.verdict.suspicious} estimatedBits=${report.estimatedBits}`,
);
