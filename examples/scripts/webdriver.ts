// Demonstrates services.webdriver — the W3C WebDriver client. Self-skips when
// no chromedriver/geckodriver is on PATH so `make demo` stays green. Uses a
// data: URL (network-free).
if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver demo.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get("data:text/html," + encodeURIComponent("<title>wd demo</title><h1 id=hi>Hello</h1><input id=box>"));
    runtime.log("title:", await d.title());
    const h1 = await d.find("id", "hi");
    runtime.log("h1 text:", await h1.text(), "visible:", await h1.isDisplayed());
    const box = await d.find("css", "#box");
    await box.sendKeys("typed by sercon");
    // getAttribute reads the HTML *attribute* (null when absent — e.g. an
    // input's `value` attribute is not the typed text, and Firefox correctly
    // returns null). Read live DOM *properties* via executeScript so it works
    // the same on Chrome and Firefox.
    runtime.log("box id attr:", await box.getAttribute("id"));            // "box"
    runtime.log("box value prop:", await d.executeScript("return document.querySelector('#box').value", []));
    runtime.log("eval 6*7:", await d.executeScript("return 6*7", []));
    const shot = await d.screenshot();
    runtime.log("screenshot bytes:", new Uint8Array(shot.bytes).length, shot.format);
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
