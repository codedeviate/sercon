// text.stego — hide a secret in cover text with zero-width characters.
const cover = "Meeting notes: ship the release on Friday.";
const out = text.stego.embed(cover, "the real date is Monday", { password: "p" });

// The visible text is unchanged; the payload rides along invisibly.
const visible = out.replace(/[​‌]/g, "");
runtime.assert.equal(visible, cover, "visible text unchanged");

const msg = text.stego.extract(out, { password: "p" });
runtime.assert.equal(msg, "the real date is Monday", "secret recovered");
runtime.log("text-stego OK: hidden", out.length - cover.length, "zero-width chars");
