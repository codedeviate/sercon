// sws6/checkout-payment.ts — Checkout / payment-provider TEMPLATE (dry-run).
//
// Adds a product, goes to /en/checkout, locates the chosen payment provider's
// UI, and STOPS before entering payment ("would proceed to <provider>"). It
// reports state and always exits 0 — a scaffold for building real provider
// flows, not a passing end-to-end purchase.
//
// Why dry-run: the provider payment UIs (KCO/SCO/Nets/Qliro) render in iframes
// with possible 3DS redirects via ngrok ("Visit Site"). Automating those in raw
// WebDriver is brittle; the Playwright-based /devshop skill is purpose-built for
// the full purchase (e.g. `/devshop buy watch-arne-jacobsen via kco`).
//
// Session note: connectShop() spoofs a normal desktop UA so the shop issues its
// `swssid` session cookie — without it the basket never persists. With it, a
// GUEST basket persists to /en/checkout (logging in is optional and only
// attempted when DEVSHOP_EMAIL/PASSWORD are set).
//
// Payment test data (card / personnr / orgnr / postal) is read from the
// ENVIRONMENT via shop.ts `paymentData()` — see sws6/.env.example. Nothing
// sensitive is hard-coded here.
//
// Usage (use `--` so sercon doesn't treat the provider as another script):
//   ./sercon examples/scripts/sws6/checkout-payment.ts            # default kco
//   ./sercon examples/scripts/sws6/checkout-payment.ts -- sco
//   ./sercon examples/scripts/sws6/checkout-payment.ts -- nets
//   ./sercon examples/scripts/sws6/checkout-payment.ts -- qliro
// runtime.argv = ["sercon", scriptPath, ...userArgs] — provider is argv[2].

import { EN, loginCreds, haveCreds, paymentData, shopUp, connectShop } from "./shop.ts";

// Per-provider DOM hints (which element/iframe marks the provider on the page).
const PROVIDERS: Record<string, { label: string; optionSelector: string; iframeSelector: string; fields: string }> = {
  kco: {
    label: "Klarna Checkout (KCO)",
    optionSelector: ".checkout-block.klarna-checkout-v3, [class*=klarna-checkout-v3]",
    iframeSelector: ".checkout-block.klarna-checkout-v3 iframe, [class*=klarna-checkout] iframe",
    fields: "card / exp / ccv (default card); click \"Fortsätt ändå\" on address-not-found; lands /checkout/thanks",
  },
  sco: {
    label: "Svea Checkout (SCO)",
    optionSelector: "[class*=svea-checkout], [id*=svea-checkout]",
    iframeSelector: "[class*=svea-checkout] iframe, [id*=svea] iframe",
    fields: "postal / personnr / orgnr / card",
  },
  nets: {
    label: "Nets Easy",
    optionSelector: "[class*=nets], [id*=nets]",
    iframeSelector: "[class*=nets-checkout] iframe, #nets-checkout-frame",
    fields: "postal / personnr / card",
  },
  qliro: {
    label: "Qliro",
    optionSelector: "[class*=qliro], [id*=qliro]",
    iframeSelector: "[class*=qliro] iframe, iframe[src*=qliro]",
    fields: "personnr / orgnr / default card",
  },
};

const PRODUCT_PATH = "/product/watch-arne-jacobsen";
const VARIANT_SEL = "select.attribute-value-select";
const BUY_BTN_SEL = "button.product-add-to-cart-action";

const provider = (runtime.argv[2] ?? "kco").toLowerCase();
if (!PROVIDERS[provider]) {
  runtime.log("Unknown provider:", provider, "— valid: kco, sco, nets, qliro. Using kco.");
}
const prov = PROVIDERS[provider] ?? PROVIDERS.kco;
const data = paymentData(provider in PROVIDERS ? provider : "kco");
// Report which test-data fields are available from the env (presence, not values).
const present = Object.keys(data).filter((k) => data[k]);
runtime.log("Provider:", prov.label, "— dry-run template (stops before entering payment)");

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping checkout-payment template.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping checkout-payment template.");
} else {
  const d = await connectShop();
  try {
    // ── Step 1: optional login (basket persists as guest too, via the UA) ────
    if (haveCreds()) {
      runtime.log("--- Step 1: log in (optional) ---");
      await d.get(EN + "/customer");
      try {
        await (await d.find("css", "#existing-account-type-radio")).click();
        await runtime.time.sleep(800);
        await (await d.find("css", "#login-email-field")).sendKeys(loginCreds.email);
        await (await d.find("css", "#login-password-field")).sendKeys(loginCreds.password);
        await (await d.find("css", "button.login-action")).click();
        await runtime.time.sleep(2000);
        const li = await d.executeScript(`return !!document.querySelector('a[href*="/customer/logout"]')`, []);
        runtime.log("logged in:", li, "(continuing either way — guest basket persists)");
      } catch (e) {
        runtime.log("login step skipped:", String(e));
      }
    } else {
      runtime.log("--- Step 1: no creds set — proceeding as guest ---");
    }

    // ── Step 2: add product to cart ──────────────────────────────────────────
    runtime.log("--- Step 2: add product to cart ---");
    await d.get(EN + PRODUCT_PATH);
    await runtime.time.sleep(1500);
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
    try { await (await d.find("css", BUY_BTN_SEL)).click(); runtime.log("clicked Buy"); } catch { runtime.log("WARN: Buy button not visible (combo out of stock?)"); }
    await runtime.time.sleep(2000);

    // ── Step 3: checkout ─────────────────────────────────────────────────────
    runtime.log("--- Step 3: /en/checkout ---");
    await d.get(EN + "/checkout");
    for (let i = 0; i < 8; i++) { await runtime.time.sleep(700); if (!(await d.executeScript(`return /basket is empty/i.test(document.body.innerText)`, []))) break; }
    const empty: boolean = await d.executeScript(`return /basket is empty|kundvagnen är tom/i.test(document.body.innerText)`, []);
    runtime.log("checkout title:", await d.title(), "— basket", empty ? "EMPTY" : "has items");

    // ── Step 4: locate provider UI (dry-run) ─────────────────────────────────
    const find = (sel: string) => d.executeScript(`return ${JSON.stringify(sel)}.split(',').some(s => document.querySelector(s.trim()))`, []);
    runtime.log("provider container found:", await find(prov.optionSelector), "/ iframe found:", await find(prov.iframeSelector));

    // ── DRY-RUN STOP ─────────────────────────────────────────────────────────
    runtime.log(`DRY-RUN STOP: would proceed to enter payment for ${prov.label}.`);
    runtime.log("payment fields for this provider:", prov.fields);
    runtime.log("test data loaded from env:", present.length ? present.join(", ") : "(none set — see sws6/.env.example)");
    runtime.log("Full iframe/3DS payment is best driven by the Playwright /devshop skill: /devshop buy watch-arne-jacobsen via " + provider);
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
