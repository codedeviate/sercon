// sws6/add-to-cart.ts — Adds a product to the cart on dev-shop.sws.local.
//
// Product: /en/product/watch-arne-jacobsen (has variant selects)
// Flow:
//   1. Open the product page.
//   2. Find all select.attribute-value-select elements (variant selects).
//   3. For each, select the first ENABLED (non-disabled) option and dispatch
//      a "change" event, then log the chosen option label.
//      NOTE: The instruction to "set selectedIndex=1" assumes index 1 is valid.
//      In practice, index 1 is often an unavailable variant; this script picks
//      the first available option instead (which may still be the default).
//   4. Confirm button.product-add-to-cart-action (Buy) is visible and enabled.
//   5. Click the Buy button.
//   6. Poll the header cart-count element until it shows a non-zero number
//      (budget ~10 s). Assert count ≥ 1.
//
// NOTE: Variant selection is REQUIRED before adding to cart — the Buy button
// is disabled until all selects have a non-default value in some shop configs.

import { EN, shopUp } from "./shop.ts";

const PRODUCT_PATH = "/product/watch-arne-jacobsen";
const VARIANT_SEL  = "select.attribute-value-select";
const BUY_BTN_SEL  = "button.product-add-to-cart-action";

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping add-to-cart demo.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping add-to-cart demo.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get(EN + PRODUCT_PATH);
    // Brief pause to let the page fully render before querying elements.
    await runtime.time.sleep(1500);
    runtime.log("page:", await d.title());

    // ── Select all variants (first enabled option + dispatch change) ─────────
    const variantCount: number = await d.executeScript(
      `return document.querySelectorAll('${VARIANT_SEL}').length`, []);
    runtime.log("variant selects found:", variantCount);

    const chosenVariants: string[] = [];
    for (let i = 0; i < variantCount; i++) {
      const label: string = await d.executeScript(`
        const sel = document.querySelectorAll('${VARIANT_SEL}')[${i}];
        if (!sel) return "NOT_FOUND";
        const opts = Array.from(sel.options);
        // Find first non-disabled option (skip placeholder/disabled ones).
        const firstEnabled = opts.find(o => !o.disabled && o.value !== "");
        const target = firstEnabled || opts[0];
        if (!target) return "NO_OPTIONS";
        sel.value = target.value;
        sel.dispatchEvent(new Event('change', { bubbles: true }));
        return target.text.trim();
      `, []);
      chosenVariants.push(label);
      runtime.log(`  variant[${i}] selected:`, label);
    }

    // Brief pause to let the UI react to variant selection.
    await runtime.time.sleep(800);

    // ── Confirm Buy button is present and enabled ────────────────────────────
    let buyBtn;
    try {
      buyBtn = await d.find("css", BUY_BTN_SEL);
      const enabled: boolean = await d.executeScript(
        `const b = document.querySelector('${BUY_BTN_SEL}');
         return b ? !b.disabled : false;`, []);
      runtime.log("Buy button present, enabled:", enabled);
      if (!enabled) {
        runtime.log("WARN: Buy button found but disabled — selected combo may be out of stock.");
        runtime.log("Check for .add-to-in-stock-notify-button as an alternative.");
      }
    } catch (e) {
      runtime.log("ERROR: could not find Buy button:", String(e));
      runtime.log("The selected variant combination may be out of stock (notify button shown instead).");
      throw e;
    }

    // ── Click Buy ────────────────────────────────────────────────────────────
    await buyBtn.click();
    runtime.log("clicked Buy button, variants:", chosenVariants.join(", "));

    // ── Poll header cart count (budget 10 s) ─────────────────────────────────
    // The cart-count element text is like "Items count: 1 ea".
    // We parse the first integer we find in it.
    let finalCount = 0;
    const deadline = Date.now() + 10_000;
    while (Date.now() < deadline) {
      await runtime.time.sleep(500);
      const raw: string = await d.executeScript(`
        const el = document.querySelector('[class*=cart-item-count]');
        return el ? el.textContent.trim() : "";
      `, []);
      if (raw) {
        const m = raw.match(/\d+/);
        if (m) {
          const n = parseInt(m[0], 10);
          if (n > 0) {
            finalCount = n;
            runtime.log("cart-count element text:", raw, "→ parsed count:", n);
            break;
          }
        }
      }
    }

    runtime.log("final cart item count:", finalCount);
    runtime.assert.ok(finalCount >= 1, "cart item count should be ≥ 1 after adding product");
    runtime.log("add-to-cart VERIFIED. Items in cart:", finalCount);

  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
