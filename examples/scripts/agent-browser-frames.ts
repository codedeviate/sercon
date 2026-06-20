// Demonstrates services.agentBrowser frame switching. frame(target) switches
// the session's frame context to an iframe (CSS selector or @ref) so the usual
// click/fill/get/snapshot operate inside it; frame("main") returns to the top
// document. Self-skips when the agent-browser CLI is absent.
//
// NOTE: agent-browser's frame command resolves the selector against the MAIN
// document and switches one level only — it does NOT descend into nested
// iframes (sequential frame() calls won't reach an inner frame). For nested /
// cross-origin frames (e.g. Klarna Checkout's inner frame), use
// services.webdriver (frameChain) — see webdriver-frames.ts.

if (!services.agentBrowser.available) {
  runtime.log("agent-browser not on PATH — skipping agent-browser-frames.");
} else {
  // Top document with one iframe (#box) holding an input we read after switching.
  const framed = `<input id="inner" value="from-inner">`;
  const page = `<!DOCTYPE html><body><div id="top">top</div>` +
    `<iframe id="box" srcdoc="${framed.replace(/"/g, "&quot;")}"></iframe></body>`;

  const b = await services.agentBrowser.launch({ headed: false });
  try {
    await b.open("data:text/html," + encodeURIComponent(page));

    // Switch into the iframe; the value read now comes from inside it.
    await b.frame("#box");
    const valRes = await b.get("value", "#inner");
    const val = (valRes as any).data?.value;
    runtime.assert.equal(val, "from-inner", "read element value inside the iframe");
    runtime.log("value inside #box:", JSON.stringify(val));

    // Back to the top document.
    await b.frame("main");
    runtime.log("agent-browser-frames OK");
  } finally {
    await b.close();
  }
}
