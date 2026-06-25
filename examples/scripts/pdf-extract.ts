// examples/scripts/pdf-extract.ts
// Demonstrates services.pdf — render/extract PDFs via poppler-utils.
// Self-skips (exit 0) when poppler is not on PATH, so it's safe in make demo.

if (!services.pdf.available) {
  runtime.log("poppler (pdftoppm) not on PATH — skipping pdf demo.");
} else {
  runtime.log("pdf backend:", services.pdf.backend, "| version:", await services.pdf.version());

  const src = "cmd/sercon/testdata/sample.pdf";
  const meta = await services.pdf.info(src);
  runtime.assert.ok(meta.pages >= 1, "at least one page");
  runtime.log("pages:", meta.pages);

  // Render page 1 to PNG bytes.
  if (services.pdf.tools.pdftoppm) {
    const img = await services.pdf.toImage(src, { page: 1, format: "png" });
    runtime.assert.equal(img.format, "png", "png format");
    runtime.assert.ok(img.bytes[0] === 0x89 && img.bytes[1] === 0x50, "PNG magic");
    runtime.log("page 1 png bytes:", img.bytes.length);
  }

  // Extract text.
  if (services.pdf.tools.pdftotext) {
    const txt = await services.pdf.toText(src);
    runtime.log("text snippet:", JSON.stringify(txt.trim().slice(0, 40)));
  }
  runtime.log("pdf demo OK");
}
