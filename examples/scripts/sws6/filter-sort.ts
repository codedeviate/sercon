// Demonstrates filter/sort on a category page via WebDriver:
//   1. GET /en/category/ladies
//   2. Capture the first 3 product tile texts (before sort)
//   3. Change #sort-by-select to "Product name Z-A" via executeScript
//   4. Wait for the listing to update
//   5. Capture the first 3 tile texts again (after sort)
//   6. Assert the ordering changed (at least the first tile differs)
//
// Self-skips cleanly when chromedriver is absent or dev-shop.sws.local is unreachable.

import { EN, shopUp } from "./shop.ts";

/** Read the text of the first `n` a.product-info tiles on the current page. */
async function firstNTileTexts(d: any, n: number): Promise<string[]> {
  const tiles = await d.findAll("css", "a.product-info");
  const out: string[] = [];
  for (let i = 0; i < Math.min(tiles.length, n); i++) {
    let name = "";
    // Try the most common name child selectors
    for (const sel of [".product-name", ".name", ".title"]) {
      try {
        const el = await tiles[i].find("css", sel);
        name = (await el.text()).trim();
        if (name) break;
      } catch (_) {}
    }
    if (!name) {
      // Fall back to the whole tile text, first line
      name = (await tiles[i].text()).trim().split("\n")[0];
    }
    out.push(name || `(tile ${i})`);
  }
  return out;
}

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping sws6/filter-sort.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping sws6/filter-sort.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // ── Navigate ──────────────────────────────────────────────────────────
    await d.get(EN + "/category/ladies");
    runtime.log("category URL:", await d.url());

    // ── Capture BEFORE ────────────────────────────────────────────────────
    const before = await firstNTileTexts(d, 3);
    runtime.log("before sort:", before.join(" | "));

    // ── Read the sort-select options so we pick a real non-default value ──
    const optionTexts: string[] = await d.executeScript(`
      const sel = document.querySelector('#sort-by-select');
      if (!sel) return [];
      return Array.from(sel.options).map(o => o.text.trim());
    `, []) as string[];
    runtime.log("sort options available:", optionTexts.join(", "));

    // Prefer "Z-A" if present; fall back to the last option that isn't the first
    let targetText = "Product name Z-A";
    if (!optionTexts.includes(targetText) && optionTexts.length > 1) {
      targetText = optionTexts[optionTexts.length - 1];
    }
    runtime.log("selecting sort:", targetText);

    // Apply the sort via executeScript (value + dispatched change event)
    const applied: boolean = await d.executeScript(`
      const sel = document.querySelector('#sort-by-select');
      if (!sel) return false;
      const opt = Array.from(sel.options).find(o => o.text.trim() === arguments[0]);
      if (!opt) return false;
      sel.value = opt.value;
      sel.dispatchEvent(new Event('change', { bubbles: true }));
      return true;
    `, [targetText]) as boolean;

    if (!applied) {
      runtime.log("WARNING: #sort-by-select not found or target option missing — trying submit");
      // Fallback: try clicking an option element directly
      try {
        const sel = await d.find("css", "#sort-by-select");
        await d.executeScript(`
          const sel = arguments[0];
          sel.selectedIndex = sel.options.length - 1;
          sel.dispatchEvent(new Event('change', { bubbles: true }));
        `, [sel]);
      } catch (_) {}
    }

    // Give the page a moment to re-render (client-side sort) or navigate
    await d.executeScript("return new Promise(r => setTimeout(r, 1500))", []);

    // ── Capture AFTER ─────────────────────────────────────────────────────
    const after = await firstNTileTexts(d, 3);
    runtime.log("after sort:", after.join(" | "));

    // ── Assert order changed ──────────────────────────────────────────────
    const changed = before[0] !== after[0] ||
                    JSON.stringify(before) !== JSON.stringify(after);
    runtime.assert.equal(changed, true, "product order should change after sort");
    runtime.log("sws6/filter-sort: PASS");
  } finally {
    await d.quit();
  }
}
