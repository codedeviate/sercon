// Advanced demo: a tour of the services.webdriver Phase 2 surface.
// Self-skips when no chromedriver/geckodriver is on PATH.
//
// Exercises, against a self-contained data: URL (no network):
//   1. Frames        — switchToFrame / switchToParentFrame
//   2. Windows/tabs  — newWindow / windowHandles / switchToWindow / closeWindow
//   3. Window rect   — getWindowRect / setWindowRect
//   4. Pointer actions — performActions (VIEWPORT coords) / releaseActions
//   5. Cookies       — setCookie / cookies / deleteAllCookies (best-effort:
//                      data: URLs have an opaque origin, so this is guarded)

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-advanced.");
} else {
  // Main page: an iframe + a fixed-position pointer target wired to update
  // #ptr on mousemove/click. The target sits at a known viewport box so the
  // pointer sequence (which uses viewport coordinates) lands inside it.
  const inner = `<!DOCTYPE html><html><body><div id="inner">inside-frame</div></body></html>`;
  const page = `
<!DOCTYPE html>
<html><head><title>Phase2</title>
<style>
  #target { position: fixed; left: 100px; top: 100px; width: 200px; height: 200px; background: #ccc; }
</style></head>
<body>
  <iframe id="f" width="300" height="120" srcdoc="${inner.replace(/"/g, "&quot;")}"></iframe>
  <div id="target"></div>
  <div id="ptr">none</div>
  <script>
    var t = document.getElementById('target');
    t.addEventListener('mousemove', function () {
      document.getElementById('ptr').textContent = 'moved';
    });
    t.addEventListener('click', function () {
      document.getElementById('ptr').textContent = 'clicked';
    });
  </script>
</body></html>`.trim();

  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // Set a known window size up front so viewport coords are predictable.
    const rect = await d.setWindowRect({ width: 1000, height: 800 });
    runtime.assert.ok(Math.abs(rect.width - 1000) <= 50, "window width ~1000");
    runtime.log("window rect:", JSON.stringify(await d.getWindowRect()));

    await d.get("data:text/html," + encodeURIComponent(page));
    runtime.assert.equal(await d.title(), "Phase2", "page title");

    // ── 1. Frames ────────────────────────────────────────────────────────
    await d.switchToFrame(0);
    const innerEl = await d.find("id", "inner");
    runtime.assert.equal(await innerEl.text(), "inside-frame", "read text inside iframe");
    await d.switchToParentFrame();
    await d.switchToDefaultContent();
    runtime.log("frames: read inside-frame OK");

    // ── 2. Windows / tabs ────────────────────────────────────────────────
    const before = await d.windowHandles();
    const original = await d.currentWindow();
    const opened = await d.newWindow("tab");
    const after = await d.windowHandles();
    runtime.assert.equal(after.length, before.length + 1, "a new tab opened");
    await d.switchToWindow(opened.handle);
    await d.get("data:text/html," + encodeURIComponent("<title>Tab2</title><p>tab</p>"));
    runtime.assert.equal(await d.title(), "Tab2", "second tab navigated");
    await d.closeWindow();
    await d.switchToWindow(original);
    runtime.assert.equal(await d.title(), "Phase2", "back on the original window");
    runtime.log("windows: opened + closed a tab OK");

    // ── 3. Pointer actions (viewport coords, not element-origin) ─────────
    await d.performActions([
      {
        type: "pointer",
        id: "mouse",
        parameters: { pointerType: "mouse" },
        actions: [
          { type: "pointerMove", duration: 0, x: 200, y: 200 }, // inside #target
          { type: "pointerDown", button: 0 },
          { type: "pointerUp", button: 0 },
        ],
      },
    ]);
    await d.releaseActions();
    const ptr = await (await d.find("id", "ptr")).text();
    runtime.assert.equal(ptr, "clicked", "pointer sequence moved + clicked the target");
    runtime.log("pointer actions: target reports", ptr);

    // ── 4. Cookies (best-effort; data: URLs have an opaque origin) ───────
    try {
      await d.setCookie({ name: "demo", value: "42" });
      const cookies = await d.cookies();
      const found = (cookies as any[]).some((c) => c.name === "demo");
      runtime.log("cookies: set/read", found ? "OK" : "(not persisted on data: URL)");
      await d.deleteAllCookies();
    } catch (e) {
      runtime.log("cookies: skipped (driver rejected on opaque origin):", String(e));
    }

    runtime.log("webdriver-advanced: all Phase 2 checks PASS");
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
