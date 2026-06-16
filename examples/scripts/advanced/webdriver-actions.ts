// Advanced demo: low-level W3C input via services.webdriver.performActions.
// Self-skips when no chromedriver/geckodriver is on PATH.
//
// This complements webdriver-advanced.ts (which uses the hover()/dragAndDrop()
// HELPERS). Here we drive the raw action API directly:
//   1. A pointer action sequence (move/down/up) using VIEWPORT coordinates —
//      pointer moves default to the viewport origin, NOT an element origin, so
//      coordinates are page-absolute. Landing inside a fixed-position box.
//   2. A key action sequence (keyDown/keyUp) that types into a focused input.
//   3. Cookies — setCookie / cookies / deleteAllCookies (best-effort; data:
//      URLs have an opaque origin, so this block is guarded).

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-actions.");
} else {
  // #target is a fixed box wired to report pointer move/click; #field is a
  // text input we type into via a key action sequence.
  const page = `
<!DOCTYPE html>
<html><head><title>Actions</title>
<style>
  body { margin: 0; }
  #target { position: fixed; left: 80px; top: 80px; width: 200px; height: 200px; background: #ccc; }
</style></head>
<body>
  <div id="target"></div>
  <div id="ptr">none</div>
  <input id="field" type="text" />
  <script>
    var t = document.getElementById('target');
    t.addEventListener('mousemove', function () { document.getElementById('ptr').textContent = 'moved'; });
    t.addEventListener('click',     function () { document.getElementById('ptr').textContent = 'clicked'; });
  </script>
</body></html>`.trim();

  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // Fix the window size so the viewport coordinates below are predictable.
    await d.setWindowRect({ width: 1000, height: 800 });
    await d.get("data:text/html," + encodeURIComponent(page));

    // ── 1. Pointer action sequence (viewport coordinates) ────────────────
    await d.performActions([
      {
        type: "pointer",
        id: "mouse",
        parameters: { pointerType: "mouse" },
        actions: [
          { type: "pointerMove", duration: 0, x: 180, y: 180 }, // inside #target's box
          { type: "pointerDown", button: 0 },
          { type: "pointerUp", button: 0 },
        ],
      },
    ]);
    await d.releaseActions();
    const ptr = await (await d.find("id", "ptr")).text();
    runtime.assert.equal(ptr, "clicked", "pointer sequence moved + clicked the target");
    runtime.log("pointer actions: target reports", ptr);

    // ── 2. Key action sequence (type into a focused input) ───────────────
    const field = await d.find("id", "field");
    await field.click(); // focus the input first
    await d.performActions([
      {
        type: "key",
        id: "kbd",
        actions: [
          { type: "keyDown", value: "h" }, { type: "keyUp", value: "h" },
          { type: "keyDown", value: "i" }, { type: "keyUp", value: "i" },
        ],
      },
    ]);
    await d.releaseActions();
    const typed = await d.executeScript("return document.getElementById('field').value", []);
    runtime.assert.equal(typed, "hi", "key sequence typed into the field");
    runtime.log("key actions: field value is", JSON.stringify(typed));

    // ── 3. Cookies (best-effort; data: URLs have an opaque origin) ───────
    try {
      await d.setCookie({ name: "demo", value: "42" });
      const cookies = await d.cookies();
      const found = (cookies as any[]).some((c) => c.name === "demo");
      runtime.log("cookies: set/read", found ? "OK" : "(not persisted on data: URL)");
      await d.deleteAllCookies();
    } catch (e) {
      runtime.log("cookies: skipped (driver rejected on opaque origin):", String(e));
    }

    runtime.log("webdriver-actions: raw pointer + key sequences PASS");
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
