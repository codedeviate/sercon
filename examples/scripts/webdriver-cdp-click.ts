// Demonstrates services.webdriver cdpClick — a trusted click on an element
// inside a NESTED CROSS-ORIGIN iframe. The W3C Element Click hit-tests to the
// parent iframe element and is intercepted ("element click intercepted") — the
// blocker for completing a Klarna Checkout "Pay order". cdpClick locates the
// element across the pierced frame tree, reads its true viewport coords via
// CDP DOM.getContentQuads, and dispatches a trusted Input.dispatchMouseEvent.
// Chrome-only; self-skips without a driver.

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-cdp-click.");
} else {
  // Inner frame served on a different port = a different origin.
  const inner = `<!DOCTYPE html><body style="margin:0">
    <div style="height:40px"></div>
    <button id="pay" onclick="window.__paid=1;this.textContent='PAID'">Pay order</button>
  </body>`;
  const inSrv = await server.http.listen({ port: 38247, routes: { "GET /": (q: any, r: any) => r.html(inner) } });
  const outSrv = await server.http.listen({ port: 38246, routes: { "GET /": (q: any, r: any) =>
    r.html(`<!DOCTYPE html><body style="margin:0"><div style="height:120px;background:#eee">header</div>` +
           `<iframe id="pay-frame" src="http://127.0.0.1:38247/" style="width:600px;height:300px;border:0"></iframe></body>`) } });

  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get("http://127.0.0.1:38246/");

    // No switchToFrame needed — cdpClick searches the whole (cross-origin) tree.
    const res = await d.cdpClick("xpath", '//button[normalize-space()="Pay order"]', { timeout: 5000 });
    runtime.log("cdpClick dispatched at", Math.round(res.x), Math.round(res.y));

    // Verify by reading the inner frame's flag (switch into it just to read).
    await d.switchToFrame("#pay-frame");
    const paid = await d.executeScript("return window.__paid || 0", []);
    runtime.assert.equal(paid, 1, "cdpClick fired the nested cross-origin button");
    await d.switchToDefaultContent();

    // Raw cdp() escape hatch: query the browser version directly.
    const ver: any = await d.cdp("Browser.getVersion", {});
    runtime.log("CDP Browser.getVersion product:", ver.product);

    runtime.log("webdriver-cdp-click OK, paid =", paid);
  } finally {
    await d.quit();
    await outSrv.close();
    await inSrv.close();
  }
}
