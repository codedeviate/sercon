// Demonstrates services.webdriver clickWhenReady + waitFor({enabled}) — the
// reliable pattern for buttons that render/enable asynchronously inside a
// (cross-origin) iframe, e.g. a Klarna Checkout "Pay order". Self-skips without
// a driver. find/element interaction already follow the active frame
// (switchToFrame/frameChain); the only thing that made such flows flaky was
// timing — clickWhenReady waits (visible+enabled) in the active frame then
// issues a native (trusted) click that fires React handlers.

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-wait-click.");
} else {
  // Inner frame (a different origin via a different port) injects a button ~500ms
  // after load, disabled, then enables it ~400ms later — mimicking a payment UI.
  const inner = `<!DOCTYPE html><body><script>
    setTimeout(() => {
      const b = document.createElement('button');
      b.id = 'pay'; b.disabled = true; b.textContent = 'Pay order';
      b.onclick = () => { window.__paid = 1; b.textContent = 'PAID'; };
      document.body.appendChild(b);
      setTimeout(() => { b.disabled = false; }, 400);
    }, 500);
  </script></body>`;
  const inSrv = await server.http.listen({ port: 38245, routes: { "GET /": (q: any, r: any) => r.html(inner) } });
  const outSrv = await server.http.listen({ port: 38244, routes: { "GET /": (q: any, r: any) =>
    r.html(`<!DOCTYPE html><body><iframe id="pay-frame" src="http://127.0.0.1:38245/"></iframe></body>`) } });

  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get("http://127.0.0.1:38244/");
    await d.switchToFrame("#pay-frame");

    // waitFor with enabled: returns only once the button is present AND enabled.
    const btn = await d.waitFor("id", "pay", { timeout: 4000, visible: true, enabled: true });
    runtime.assert.equal(await btn.text(), "Pay order", "button ready");

    // Reset for the one-call form: reload the frame and use clickWhenReady.
    await d.switchToDefaultContent();
    await d.get("http://127.0.0.1:38244/");
    await d.switchToFrame("#pay-frame");
    await d.clickWhenReady("id", "pay", { timeout: 4000 });   // waits visible+enabled, trusted-clicks
    const paid = await d.executeScript("return window.__paid || 0", []);
    runtime.assert.equal(paid, 1, "clickWhenReady fired the handler");
    runtime.log("clickWhenReady paid =", paid);

    runtime.log("webdriver-wait-click OK");
  } finally {
    await d.quit();
    await outSrv.close();
    await inSrv.close();
  }
}
