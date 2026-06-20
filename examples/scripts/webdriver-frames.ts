// Demonstrates services.webdriver nested-frame addressing: frameChain([...])
// reaches a deeply nested iframe in one call, and the selector form of
// switchToFrame scopes subsequent queries to that frame. Self-skips without a
// driver. Nested same-origin iframes via data: URLs — the same W3C /frame path
// the driver uses for cross-origin frames (e.g. a Klarna Checkout widget).

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-frames.");
} else {
  const inner = `<!DOCTYPE html><body><div id="inner">deep</div></body>`;
  const outer = `<!DOCTYPE html><body><iframe id="if-inner" src="data:text/html,${encodeURIComponent(inner)}"></iframe></body>`;
  const page = `<!DOCTYPE html><body><div id="top">top</div>` +
    `<iframe id="if-outer" src="data:text/html,${encodeURIComponent(outer)}"></iframe></body>`;

  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get("data:text/html," + encodeURIComponent(page));

    // One-call nesting: switch from the top document through each level.
    await d.frameChain(["#if-outer", "#if-inner"]);
    const deep = await (await d.find("id", "inner")).text();
    runtime.assert.equal(deep, "deep", "frameChain reached the inner frame");
    runtime.log("frameChain read inner frame:", JSON.stringify(deep));

    // Reset, then the selector form of switchToFrame (one level). Queries are
    // now scoped to that frame.
    await d.switchToDefaultContent();
    await d.switchToFrame("#if-outer");
    const innerIframes = await d.findAll("id", "if-inner");
    runtime.assert.equal(innerIframes.length, 1, "switchToFrame(selector) scoped to the outer frame");
    runtime.log("switchToFrame(selector): inner iframe count =", innerIframes.length);

    await d.switchToDefaultContent();
    runtime.log("webdriver-frames OK");
  } finally {
    await d.quit();
  }
}
