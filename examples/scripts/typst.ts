// Demonstrates services.typst — compile Typst docs via the external `typst` CLI.
// Self-skips (exit 0) when typst is not on PATH, so it's safe in make demo.

if (!services.typst.available) {
  runtime.log("typst not on PATH — skipping typst demo.");
} else {
  runtime.log("typst:", await services.typst.version());

  // Compile inline source to a PDF (returned as bytes).
  const doc = "= Hello from sercon\nThis PDF was typeset by Typst.";
  const pdf = await services.typst.compile({ source: doc });
  runtime.assert.equal(pdf.format, "pdf", "pdf format");
  // %PDF magic.
  runtime.assert.ok(pdf.bytes[0] === 37 && pdf.bytes[1] === 80, "PDF magic %P");
  runtime.log("pdf bytes:", pdf.bytes.length);

  // Render to a PNG on disk.
  const tmp = (runtime.env.get("TMPDIR") ?? "/tmp") + "/sercon-typst-demo.png";
  const png = await services.typst.compile({ source: doc, output: tmp, ppi: 96 });
  runtime.assert.equal(png.path, tmp, "png written to path");
  runtime.log("png written:", png.path);

  // query a labeled metadata value.
  const v = await services.typst.query({
    source: '#metadata(42) <answer>', selector: "<answer>", field: "value", one: true,
  });
  runtime.assert.equal(v, 42, "query metadata value");
  runtime.log("query <answer>.value =", v, "| fonts:", (await services.typst.fonts()).length);
  runtime.log("typst demo OK");
}
