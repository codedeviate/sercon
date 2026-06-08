// Demonstrates services.webdriver Phase 2 — windows/tabs, frames, alerts,
// window rect, action chains (hover/drag), and executeScript element handles.
// Self-skips when no chromedriver/geckodriver is on PATH so `make demo` stays
// green. Network-free (data: URLs).
if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-advanced demo.");
} else {
  const d = await services.webdriver.connect({
    browser: "chrome",
    headless: true,
    capabilities: { unhandledPromptBehavior: "ignore" },
  });
  try {
    // Windows / tabs
    const before = (await d.windowHandles()).length;
    const nw = await d.newWindow("tab");
    await d.switchToWindow(nw.handle);
    runtime.log("tabs:", before, "->", (await d.windowHandles()).length);
    await d.closeWindow();

    // Window rect
    await d.setWindowRect({ width: 900, height: 700 });
    const r = await d.getWindowRect();
    runtime.log("rect:", r.width + "x" + r.height);

    // Frames
    await d.get("data:text/html," + encodeURIComponent(
      "<title>outer</title><iframe srcdoc='<p id=inner>hi</p>'></iframe>"));
    await d.switchToFrame(0);
    runtime.log("frame text:", await (await d.find("id", "inner")).text());
    await d.switchToParentFrame();
    await d.switchToDefaultContent();

    // Alerts — navigate to a page that fires alert() synchronously on load.
    // get() blocks until the page is "loaded"; with unhandledPromptBehavior:
    // "ignore" the alert remains open, so alertText() and acceptAlert() work.
    try {
      await d.get("data:text/html," + encodeURIComponent(
        "<body onload=\"alert('hi from alert')\"><title>a</title>"));
    } catch (_) {
      // chromedriver may return an unexpected-alert-open error for the get()
      // itself — the alert is still live, carry on.
    }
    runtime.log("alert:", await d.alertText());
    await d.acceptAlert();

    // Actions: hover + drag (events tracked via a listener)
    await d.get("about:blank");
    await d.executeScript(`
      document.body.style.margin='0';
      document.body.innerHTML='<div id=x style="position:absolute;top:60px;left:60px;width:100px;height:100px"></div>'
        + '<div id=y style="position:absolute;top:60px;left:300px;width:100px;height:100px"></div>';
      window.__log=[];
      for (const id of ['x','y']) {
        const el=document.getElementById(id);
        ['mouseover','mousedown','mouseup'].forEach(e=>el.addEventListener(e,()=>window.__log.push(id+':'+e)));
      }
      return 1;`, []);
    const x = await d.find("id", "x");
    const y = await d.find("id", "y");
    await x.hover();
    await d.dragAndDrop(x, y);
    runtime.log("events:", await d.executeScript("return (window.__log||[]).join(' ')", []));

    // executeScript returning an element handle
    const el = await d.executeScript("return document.getElementById('y')", []);
    runtime.log("script element tag:", await el.tagName());
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
