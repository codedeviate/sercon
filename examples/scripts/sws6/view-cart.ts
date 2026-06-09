// sws6/view-cart.ts — Add a product, then view it in the cart at /en/checkout.
//
// Flow:
//   1. Open a product, select its variants, click Buy, confirm the header
//      cart count goes to ≥1.
//   2. Navigate to /en/checkout and wait for the cart to render.
//   3. Assert the basket is NOT empty and contains the product, and log the
//      order total.
//
// Session note: connectShop() spoofs a normal desktop UA so the shop issues
// its `swssid` session cookie — that's what makes the basket persist from the
// product page to /en/checkout (even as a guest). With the default
// HeadlessChrome UA the shop withholds the session and the cart looks empty.
//
// Self-skips when no driver is on PATH or the host is unreachable.

import { EN, shopUp, connectShop } from "./shop.ts";

const PRODUCT_PATH = "/product/watch-arne-jacobsen";
const PRODUCT_MATCH = /watch|armbandsur|arne/i;
const VARIANT_SEL = "select.attribute-value-select";
const BUY_BTN_SEL = "button.product-add-to-cart-action";

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping view-cart demo.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping view-cart demo.");
} else {
  const d = await connectShop();
  try {
    // ── 1. add the product ───────────────────────────────────────────────────
    await d.get(EN + PRODUCT_PATH);
    await runtime.time.sleep(1500);
    runtime.log("product page:", await d.title());

    const variantCount: number = await d.executeScript(`return document.querySelectorAll('${VARIANT_SEL}').length`, []);
    for (let i = 0; i < variantCount; i++) {
      await d.executeScript(`
        const sel = document.querySelectorAll('${VARIANT_SEL}')[${i}];
        if (!sel) return;
        const t = Array.from(sel.options).find(o => !o.disabled && o.value !== "") || sel.options[0];
        if (t) { sel.value = t.value; sel.dispatchEvent(new Event('change', { bubbles: true })); }
      `, []);
    }
    await runtime.time.sleep(800);
    await (await d.find("css", BUY_BTN_SEL)).click();
    runtime.log("clicked Buy");

    let headerCount = 0;
    const deadline = Date.now() + 10_000;
    while (Date.now() < deadline) {
      await runtime.time.sleep(500);
      const raw: string = await d.executeScript(`const el = document.querySelector('[class*=cart-item-count]'); return el ? el.textContent.trim() : ""`, []);
      const m = raw.match(/\d+/);
      if (m && parseInt(m[0], 10) > 0) { headerCount = parseInt(m[0], 10); break; }
    }
    runtime.assert.ok(headerCount >= 1, "expected header cart count ≥ 1 after Buy");
    runtime.log("header cart count after add:", headerCount);

    // ── 2. view the cart at /en/checkout ─────────────────────────────────────
    await d.get(EN + "/checkout");
    let empty = true;
    for (let i = 0; i < 10; i++) {
      await runtime.time.sleep(700);
      empty = await d.executeScript(`return /basket is empty|kundvagnen är tom/i.test(document.body.innerText)`, []);
      if (!empty) break;
    }
    runtime.log("checkout title:", await d.title());

    // ── 3. assert the item is in the cart ────────────────────────────────────
    const state = await d.executeScript(`
      const t = (document.body.innerText || "").replace(/\\s+/g, " ");
      const total = (t.match(/(\\d[\\d\\s]*(?:kr|sek))/i) || [])[1] || "";
      return JSON.stringify({ empty: /basket is empty|kundvagnen är tom/i.test(t), hasProduct: ${PRODUCT_MATCH}.test(t), total });
    `, []);
    const s = JSON.parse(state as string);
    runtime.log("cart:", state);
    runtime.assert.ok(!s.empty, "basket should not be empty at /en/checkout (session via connectShop UA)");
    runtime.assert.ok(s.hasProduct, "the added product should appear in the cart");
    runtime.log("✓ cart shows the product. Order total:", s.total || "(not parsed)");
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
