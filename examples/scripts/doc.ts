// Demonstrates codec.doc — author a DOCX, read it back, and show the read/write
// capability matrix. Offline + self-contained; runs in make demo.

const out = codec.doc.write({ paragraphs: ["Hello from sercon", "Second paragraph"] }, { format: "docx" });
runtime.assert.ok(out.bytes.length > 0, "docx bytes produced");

const back = codec.doc.read(out.bytes, { format: "docx" });
runtime.assert.equal(back.format, "docx", "round-trips as docx");
runtime.assert.equal(back.paragraphs.length, 2, "two paragraphs");
runtime.assert.equal(back.paragraphs[0], "Hello from sercon", "first paragraph survives");

const f = codec.doc.formats();
runtime.assert.ok(f.pdf.read && !f.pdf.write, "pdf is read-only");
runtime.assert.ok(f.docx.read && f.docx.write, "docx is read+write");

runtime.log("doc OK: DOCX round-trip; formats:", Object.keys(f).join(","));
