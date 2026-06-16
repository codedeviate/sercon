// Advanced demo: drive a REMOTE WebDriver endpoint (Selenium Grid or a
// standalone driver started elsewhere) instead of spawning a local one.
// Self-skips when no grid URL is configured or the endpoint isn't ready —
// so it's safe to run anywhere and stays out of CI.
//
// To try it for real, stand up an endpoint and point the env var at it:
//
//   # Standalone chromedriver on a fixed port:
//   chromedriver --port=4444 --allowed-ips=127.0.0.1
//   export SERCON_WEBDRIVER_GRID_URL=http://127.0.0.1:4444
//
//   # Or a Selenium Grid (its WebDriver base path is /wd/hub):
//   docker run -d -p 4444:4444 selenium/standalone-chrome
//   export SERCON_WEBDRIVER_GRID_URL=http://127.0.0.1:4444/wd/hub
//
// Then: sercon examples/scripts/advanced/webdriver-grid.ts

const url = runtime.env.get("SERCON_WEBDRIVER_GRID_URL");
if (!url) {
  runtime.log("SERCON_WEBDRIVER_GRID_URL unset — skipping webdriver-grid.");
} else {
  // probe() never throws on transport errors — it returns { ready: false }.
  const status = await services.webdriver.probe({ url });
  if (!status.ready) {
    runtime.log(`grid at ${url} not ready (${status.error || status.status}) — skipping.`);
  } else {
    runtime.log(`grid ready at ${url} (HTTP ${status.status})`);
    // connect({ url }) dials the already-running remote driver rather than
    // starting a local binary. capabilities is the raw W3C escape hatch,
    // merged last — here we just request a headless Chrome.
    const d = await services.webdriver.connect({
      url,
      browser: "chrome",
      headless: true,
      capabilities: {
        "goog:chromeOptions": { args: ["--headless=new", "--no-sandbox"] },
      },
    });
    try {
      await d.get("data:text/html," + encodeURIComponent("<title>Grid OK</title><h1>remote</h1>"));
      runtime.assert.equal(await d.title(), "Grid OK", "remote session navigated");
      runtime.log("remote title:", await d.title());
      runtime.log("webdriver-grid: remote session PASS");
    } finally {
      await d.quit();
      runtime.log("remote session quit.");
    }
  }
}
