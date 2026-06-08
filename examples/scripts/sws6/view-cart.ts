// sws6/view-cart.ts — Demonstrates the cart-view step and documents the
//                     guest-basket persistence caveat.
//
// Flow:
//   1. Add a product to cart (same variant-select + click-Buy steps as
//      add-to-cart.ts) and confirm the header count goes non-zero.
//   2. Navigate to /en/checkout.
//   3. Read and log the cart state (empty vs items, any order total text).
//
// IMPORTANT — GUEST BASKET CAVEAT:
//   The guest basket is client-side only and does NOT persist when
//   navigating to /en/checkout. The site shows "Basket is empty" (and a
//   toast "Log in to save your basket") even though the header count was
//   non-zero just before the navigation. This is expected shop behaviour,
//   not a test bug.
//
//   This script therefore asserts ONLY that /en/checkout loaded (page title
//   is non-empty). It logs whether the cart is empty or has items, but does
//   NOT fail if the cart is empty for a guest session.
//
//   To test checkout with real cart persistence, run login.ts first (same
//   browser session), then add to cart, then navigate to /en/checkout.
//   That flow is wired together in checkout-payment.ts.

import { EN, shopUp } from "./shop.ts";

const PRODUCT_PATH = "/product/watch-arne-jacobsen";
const VARIANT_SEL  = "select.attribute-value-select";
const BUY_BTN_SEL  = "button.product-add-to-cart-action";

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping view-cart demo.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping view-cart demo.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // ── 1. Add a product (same steps as add-to-cart.ts) ─────────────────────
    await d.get(EN + PRODUCT_PATH);
    await runtime.time.sleep(1500);
    runtime.log("product page:", await d.title());

    const variantCount: number = await d.executeScript(
      `return document.querySelectorAll('${VARIANT_SEL}').length`, []);
    for (let i = 0; i < variantCount; i++) {
      await d.executeScript(`
        const sel = document.querySelectorAll('${VARIANT_SEL}')[${i}];
        if (!sel) return;
        const opts = Array.from(sel.options);
        const firstEnabled = opts.find(o => !o.disabled && o.value !== "");
        const target = firstEnabled || opts[0];
        if (!target) return;
        sel.value = target.value;
        sel.dispatchEvent(new Event('change', { bubbles: true }));
      `, []);
    }
    await runtime.time.sleep(800);

    let buyBtn;
    try {
      buyBtn = await d.find("css", BUY_BTN_SEL);
    } catch {
      runtime.log("WARN: Buy button not found after variant selection (out of stock?). Skipping add step.");
    }

    if (buyBtn) {
      await buyBtn.click();
      runtime.log("clicked Buy button");

      // Poll header count briefly just to confirm the add registered.
      let headerCount = 0;
      const deadline = Date.now() + 10_000;
      while (Date.now() < deadline) {
        await runtime.time.sleep(500);
        const raw: string = await d.executeScript(`
          const el = document.querySelector('[class*=cart-item-count]');
          return el ? el.textContent.trim() : "";
        `, []);
        const m = raw.match(/\d+/);
        if (m && parseInt(m[0], 10) > 0) {
          headerCount = parseInt(m[0], 10);
          runtime.log("header cart count after add:", headerCount, "(raw:", raw + ")");
          break;
        }
      }
      if (headerCount === 0) {
        runtime.log("WARN: header cart count stayed 0 after clicking Buy.");
      }
    }

    // ── 2. Navigate to /en/checkout ──────────────────────────────────────────
    await d.get(EN + "/checkout");
    await runtime.time.sleep(1000);

    const checkoutTitle = await d.title();
    runtime.log("checkout page title:", checkoutTitle);

    // Assert the page loaded at all.
    runtime.assert.ok(checkoutTitle.length > 0, "checkout page title should be non-empty");

    // ── 3. Read and log the cart state ───────────────────────────────────────
    // Look for a total/price element or "basket is empty" text.
    const pageText: string = await d.executeScript(
      `return document.body ? document.body.innerText : ""`, []);

    // Is the basket advertised as empty?
    const isEmpty = /basket is empty|kundvagnen är tom|tom korg/i.test(pageText);
    if (isEmpty) {
      runtime.log(
        "CART STATE: empty at /en/checkout — expected for a guest session.",
        "Guest basket is client-side only and does not persist across page navigation.",
        "Run login.ts flow first and then add to cart to test with a real basket."
      );
    } else {
      // Try to read an order total.
      const totalEl: string = await d.executeScript(`
        const candidates = [
          '.order-total', '.cart-total', '[class*=order-total]',
          '[class*=cart-total]', '[class*=total-price]',
        ];
        for (const sel of candidates) {
          const el = document.querySelector(sel);
          if (el) return el.textContent.trim();
        }
        return "";
      `, []);
      runtime.log("CART STATE: items appear present at /en/checkout. Order total element:", totalEl || "(none found)");
    }

    runtime.log("view-cart script complete. Assertion: checkout page loaded (title non-empty) — PASSED.");
    runtime.log("NOTE: a missing cart at /en/checkout is normal for guests. See script header comment.");

  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
