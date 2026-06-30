// Recipe: convert a document up — author an RTF, extract its text, then re-emit
// it as DOCX in $TMPDIR (the "get to the information, then convert" workflow).
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

// Synthesize a small RTF document (a read+write format).
const rtf = codec.doc.write({ paragraphs: ["Quarterly report", "Revenue is up."] }, { format: "rtf" });

// Extract its text — works the same for pdf/doc/docx/odt sources.
const doc = codec.doc.read(rtf.bytes, { format: "rtf" });
runtime.assert.equal(doc.paragraphs.length, 2, "extracted two paragraphs");
runtime.assert.ok(doc.text.includes("Revenue is up."), "text extracted");

// Convert up to DOCX.
const docxPath = `${tmp}/sercon-doc.docx`;
await fs.writeBytes(docxPath, codec.doc.write(doc, { format: "docx" }).bytes);
const back = codec.doc.read(await fs.readBytes(docxPath));
runtime.assert.equal(back.paragraphs.length, 2, "round-trips through docx");

runtime.log(`doc-extract: RTF→text→DOCX at ${docxPath} — paragraphs: ${doc.paragraphs.length}`);
