// Demonstrates services.webdriver against a TRUE out-of-process iframe (OOPIF).
// cdpClick (v0.61.0) routes input over a browser-level CDP connection, so it
// fires a button inside a cross-*site* iframe — the real Klarna Checkout case,
// which v0.60.0's same-site two-port fixture could not reach. Also shows the
// targets()/attach() API. Chrome-only; self-skips without a driver.
//
// A genuine OOPIF needs a cross-SITE (different eTLD+1) iframe, not just a
// different port. We map a.test/b.test -> 127.0.0.1 and force site isolation.
// The iframe is injected ~800ms AFTER load to exercise target re-enumeration.

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-cdp-oopif.");
} else {
  const inner = `<!DOCTYPE html><body style="margin:0"><div style="height:30px"></div>
    <button id="pay" onclick="window.__hit=(window.__hit||0)+1;this.textContent='PAID'">Pay order</button></body>`;
  const inSrv = await server.http.listen({ port: 38249, routes: { "GET /": (q: any, r: any) => r.html(inner) } });
  // Outer page injects the cross-site iframe ~800ms after load (late OOPIF).
  const outer = `<!DOCTYPE html><body style="margin:0"><div style="height:100px;background:#ccc">header</div>
    <div id="slot"></div><script>
      setTimeout(() => {
        const f = document.createElement('iframe');
        f.src = 'http://b.test:38249/';
        f.style.cssText = 'width:500px;height:300px;border:0';
        document.getElementById('slot').appendChild(f);
      }, 800);
    </script></body>`;
  const outSrv = await server.http.listen({ port: 38248, routes: { "GET /": (q: any, r: any) => r.html(outer) } });

  const d = await services.webdriver.connect({
    browser: "chrome", headless: true,
    args: ["--host-resolver-rules=MAP *.test 127.0.0.1", "--site-per-process"],
  });
  try {
    await d.get("http://a.test:38248/");

    // cdpClick polls + re-enumerates targets, so it waits for the late OOPIF,
    // then fires the button inside it (page-session input cannot reach it).
    const res = await d.cdpClick("xpath", '//button[normalize-space()="Pay order"]', { timeout: 8000 });
    runtime.log("cdpClick dispatched at", Math.round(res.x), Math.round(res.y), "target", res.targetId);

    // The OOPIF target should now be present; assert it and verify via its session.
    const targets = await d.targets();
    const oopif = targets.find((t: any) => t.type === "iframe" && /b\.test/.test(t.url));
    runtime.assert.ok(oopif, "expected an out-of-process iframe target for b.test");
    runtime.log("OOPIF target:", oopif.url);

    const sess = await d.attach(oopif);
    const ev: any = await sess.cdp("Runtime.evaluate", { expression: "window.__hit||0", returnByValue: true });
    runtime.assert.equal(ev.result.value, 1, "cdpClick fired the OOPIF button");
    await sess.detach();

    runtime.log("webdriver-cdp-oopif OK, __hit =", ev.result.value);
  } finally {
    await d.quit();
    await outSrv.close();
    await inSrv.close();
  }
}
