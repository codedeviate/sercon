// Demonstrates services.agentBrowser Phase 2 — capture, settings, defaults,
// and the flat one-shot shortcuts. Self-skips when the CLI is absent so
// `make demo` stays green. Network-free (data: URLs).

if (!services.agentBrowser.available) {
  runtime.log("agent-browser CLI not found on PATH — skipping demo.");
} else {
  const html = "data:text/html," + encodeURIComponent(
    "<title>capture demo</title><h1 id=hi>Hello</h1>"
  );

  // Namespace-level defaults flow into every launch().
  services.agentBrowser.setDefaultOptions({ headed: false });
  runtime.log("defaults:", JSON.stringify(services.agentBrowser.defaultOptions()));

  const b = services.agentBrowser.launch();
  try {
    await b.open(html);
    await b.set.viewport(1024, 768);

    // Capture to bytes (no path) — bytes is a number[] in this engine.
    const shot = await b.screenshot({ full: true });
    runtime.log("screenshot bytes:", new Uint8Array(shot.bytes).length, shot.format);

    // Capture to a file.
    const file = await b.screenshot("/tmp/sercon-capture-demo.png");
    runtime.log("screenshot file:", JSON.stringify(file));

    // PDF to bytes.
    const pdf = await b.pdf();
    runtime.log("pdf bytes:", new Uint8Array(pdf.bytes).length, pdf.format);
  } finally {
    await b.close();
  }

  // One-shot shortcut: launch+open+act+close in a single call.
  const r = await services.agentBrowser.eval(html, "document.title");
  runtime.log("one-shot eval:", JSON.stringify(r.data ?? r));

  services.agentBrowser.clearDefaultOptions();
  runtime.log("done.");
}
