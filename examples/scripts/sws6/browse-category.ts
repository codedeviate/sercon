// Demonstrates browsing a storefront category page via WebDriver:
//   1. GET /en/category/ladies
//   2. Assert >1 product tile (a.product-info) is listed (expect ~12)
//   3. Log the first few product names
//   4. Click the first tile and assert a product page loads
//
// Self-skips cleanly when chromedriver is absent or dev-shop.sws.local is unreachable.

import { BASE, EN, shopUp, connectShop } from "./shop.ts";

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping sws6/browse-category.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping sws6/browse-category.");
} else {
  const d = await connectShop();
  try {
    // ── Navigate to ladies category ───────────────────────────────────────
    await d.get(EN + "/category/ladies");
    runtime.log("category URL:", await d.url());

    // ── Assert tile count ─────────────────────────────────────────────────
    const tiles = await d.findAll("css", "a.product-info");
    runtime.assert.ok(tiles.length > 1, "expected >1 product tile on ladies category page, got: " + tiles.length);
    runtime.log("product tiles found:", tiles.length);

    // ── Log first few product names ───────────────────────────────────────
    const previewCount = Math.min(tiles.length, 4);
    const names: string[] = [];
    for (let i = 0; i < previewCount; i++) {
      try {
        // Product name is typically in a child .product-name / .name span,
        // or falls back to the title attribute on the anchor itself.
        let name = "";
        try {
          const nameEl = await tiles[i].find("css", ".product-name");
          name = (await nameEl.text()).trim();
        } catch (_) {}
        if (!name) {
          try {
            const nameEl = await tiles[i].find("css", ".name");
            name = (await nameEl.text()).trim();
          } catch (_) {}
        }
        if (!name) {
          name = (await tiles[i].getAttribute("title") ?? "").trim();
        }
        if (!name) {
          name = (await tiles[i].text()).trim().split("\n")[0];
        }
        names.push(name || `(tile ${i})`);
      } catch (_) {
        names.push(`(tile ${i} — name unavailable)`);
      }
    }
    runtime.log("first few products:", names.join(" | "));

    // ── Click first tile ──────────────────────────────────────────────────
    const firstHref = await tiles[0].getAttribute("href");
    runtime.log("clicking first tile:", firstHref);
    await tiles[0].click();

    const productUrl = await d.url();
    runtime.log("landed on:", productUrl);

    // Assert product page loaded (URL contains /product/ or the page has a title)
    const pageTitle = await d.title();
    runtime.assert.ok(pageTitle !== "", "product page title must not be empty");
    runtime.log("product page title:", pageTitle);

    runtime.log("sws6/browse-category: PASS");
  } finally {
    await d.quit();
  }
}
