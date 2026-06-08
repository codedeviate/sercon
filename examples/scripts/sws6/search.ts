// Demonstrates searching the dev-shop storefront via WebDriver:
//   1. GET /en/search?q=watch
//   2. Assert ≥1 a.product-info result is returned
//   3. Click the first result link
//   4. Assert we land on a product page (URL contains /product/ AND title non-empty)
//
// Self-skips cleanly when chromedriver is absent or dev-shop.sws.local is unreachable.

import { BASE, EN, shopUp } from "./shop.ts";

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping sws6/search.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping sws6/search.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // ── Search ─────────────────────────────────────────────────────────────
    await d.get(EN + "/search?q=watch");
    runtime.log("search URL:", await d.url());

    // Collect all product-info links
    const results = await d.findAll("css", "a.product-info");
    runtime.assert.ok(results.length > 0, "expected ≥1 search result for 'watch'");
    runtime.log("search results found:", results.length);

    // Grab the href of the first result so we can verify later
    const firstHref = await results[0].getAttribute("href");
    runtime.log("first result href:", firstHref);

    // ── Click first result ────────────────────────────────────────────────
    await results[0].click();
    const productUrl = await d.url();
    runtime.log("landed on:", productUrl);

    // ── Assert product page ───────────────────────────────────────────────
    runtime.assert.ok(productUrl.includes("/product/"), "URL should contain /product/ — got: " + productUrl);

    const pageTitle = await d.title();
    runtime.assert.ok(pageTitle !== "", "product page title must not be empty");
    runtime.log("product page title:", pageTitle);

    // Also grab the visible <h1> if present
    try {
      const h1 = await d.find("css", "h1");
      const h1Text = await h1.text();
      if (h1Text) runtime.log("product h1:", h1Text);
    } catch (_) {
      // h1 may not exist — not a hard requirement here
    }

    runtime.log("sws6/search: PASS");
  } finally {
    await d.quit();
  }
}
