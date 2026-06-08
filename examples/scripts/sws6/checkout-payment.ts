// sws6/checkout-payment.ts — Checkout / payment provider TEMPLATE (dry-run).
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │  THIS IS A SCAFFOLD, NOT A PASSING END-TO-END TEST.                    │
// │                                                                         │
// │  • Guest basket does NOT persist to /en/checkout — a real run must     │
// │    log in first (same session as login.ts) then add to cart.           │
// │  • Provider payment UIs are iframes + possible 3DS redirects via       │
// │    ngrok ("Visit Site" button). Full automation of those flows is       │
// │    brittle in raw WebDriver and is best driven by the Playwright-based  │
// │    /devshop skill (e.g. `/devshop buy ... via kco`).                   │
// │  • This script goes as far as it can reliably: add item → /en/checkout │
// │    → locate provider UI → STOP and log "would proceed to <provider>".  │
// │  • Exit is always 0; the script reports state rather than asserting it. │
// └─────────────────────────────────────────────────────────────────────────┘
//
// Usage:
//   /tmp/sercon-sws6b -timeout 90s examples/scripts/sws6/checkout-payment.ts
//   /tmp/sercon-sws6b -timeout 90s examples/scripts/sws6/checkout-payment.ts -- kco
//   /tmp/sercon-sws6b -timeout 90s examples/scripts/sws6/checkout-payment.ts -- sco
//   /tmp/sercon-sws6b -timeout 90s examples/scripts/sws6/checkout-payment.ts -- nets
//   /tmp/sercon-sws6b -timeout 90s examples/scripts/sws6/checkout-payment.ts -- qliro
// Note: use `--` before user args to stop sercon treating them as extra scripts.
//       runtime.argv = ["sercon", scriptPath, ...userArgs] — provider is argv[2].
//
// Accepted providers: kco | sco | nets | qliro  (default: kco)
//
// ── Provider test data (for manual / Playwright use) ─────────────────────────
//
// KCO (Klarna Checkout):
//   Persona:  firstName="Tess" lastName="T Person" email=persona.email
//   Card:     ***REMOVED***, exp 12/34, CCV 567
//   Redirect: lands /checkout/thanks
//   Note:     click "Fortsätt ändå" if Klarna shows an address-not-found prompt.
//
// SCO (Svea Checkout):
//   Postal:   99999
//   Personnr: ***REMOVED*** (private person)
//   Orgnr:    ***REMOVED*** (company)
//   Card:     ***REMOVED***
//
// Nets Easy:
//   Postal:   83162
//   Personnr: ***REMOVED***  (alt: ***REMOVED***)
//   Card:     ***REMOVED***
//
// Qliro:
//   Personnr: ***REMOVED***
//   Orgnr:    ***REMOVED***
//   Card:     default card (Qliro test sandbox default)
//
// ─────────────────────────────────────────────────────────────────────────────

import { EN, loginCreds, shopUp } from "./shop.ts";

// ── Provider selector config ──────────────────────────────────────────────────
const PROVIDERS: Record<string, {
  label: string;
  iframeSelector: string;
  optionSelector: string;
  notes: string;
}> = {
  kco: {
    label: "Klarna Checkout (KCO)",
    iframeSelector: ".checkout-block.klarna-checkout-v3 iframe, [class*=klarna-checkout] iframe",
    optionSelector: ".checkout-block.klarna-checkout-v3, [class*=klarna-checkout-v3]",
    notes: 'Enter card ***REMOVED*** / 12/34 / 567 inside the KCO iframe. ' +
           'Click "Fortsätt ändå" if address-not-found appears. Should land /checkout/thanks.',
  },
  sco: {
    label: "Svea Checkout (SCO)",
    iframeSelector: "[class*=svea-checkout] iframe, [id*=svea] iframe",
    optionSelector: "[class*=svea-checkout], [id*=svea-checkout]",
    notes: "Enter postal 99999, personnr ***REMOVED*** (or orgnr ***REMOVED***), card ***REMOVED***.",
  },
  nets: {
    label: "Nets Easy",
    iframeSelector: "[class*=nets-checkout] iframe, #nets-checkout-frame",
    optionSelector: "[class*=nets], [id*=nets]",
    notes: "Enter postal 83162, personnr ***REMOVED*** (alt ***REMOVED***), card ***REMOVED***.",
  },
  qliro: {
    label: "Qliro",
    iframeSelector: "[class*=qliro] iframe, iframe[src*=qliro]",
    optionSelector: "[class*=qliro], [id*=qliro]",
    notes: "Enter personnr ***REMOVED*** (or orgnr ***REMOVED***), default Qliro test card.",
  },
};

const PRODUCT_PATH = "/product/watch-arne-jacobsen";
const VARIANT_SEL  = "select.attribute-value-select";
const BUY_BTN_SEL  = "button.product-add-to-cart-action";

// runtime.argv layout: [0]="sercon" [1]=script-path [2+]=user args
const provider = (runtime.argv[2] ?? "kco").toLowerCase();
if (!PROVIDERS[provider]) {
  runtime.log("Unknown provider:", provider, "— valid values: kco, sco, nets, qliro. Using kco.");
}
const prov = PROVIDERS[provider] ?? PROVIDERS.kco;
runtime.log("Provider:", prov.label, "(dry-run template — will STOP before entering payment)");

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping checkout-payment template.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping checkout-payment template.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // ── Step 1: Log in (same approach as login.ts) ───────────────────────────
    // A real run must authenticate so the basket persists to /en/checkout.
    runtime.log("--- Step 1: log in ---");
    await d.get(EN + "/customer");
    try {
      const radio = await d.find("css", "#existing-account-type-radio");
      await radio.click();
    } catch {
      runtime.log("WARN: #existing-account-type-radio not found — proceeding without click.");
    }
    // Brief pause to let the login fields reveal after clicking the radio.
    await runtime.time.sleep(800);
    const emailField = await d.find("css", "#login-email-field");
    await emailField.sendKeys(loginCreds.email);
    const pwField = await d.find("css", "#login-password-field");
    await pwField.sendKeys(loginCreds.password);
    const loginBtn = await d.find("css", "button.login-action");
    await loginBtn.click();
    await runtime.time.sleep(2000);
    runtime.log("login submitted. url:", await d.url());
    // (See login.ts for notes on verifying auth state — we proceed regardless.)

    // ── Step 2: Add product to cart ──────────────────────────────────────────
    runtime.log("--- Step 2: add product to cart ---");
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
      runtime.log("WARN: Buy button not visible after variant selection. Combo may be out of stock.");
    }
    if (buyBtn) {
      await buyBtn.click();
      runtime.log("clicked Buy");
      await runtime.time.sleep(1500);
    }

    // Header count (best-effort; may be 0 for guests even after clicking Buy)
    const raw: string = await d.executeScript(`
      const el = document.querySelector('[class*=cart-item-count]');
      return el ? el.textContent.trim() : "";
    `, []);
    runtime.log("header cart count after add:", raw || "(not found)");

    // ── Step 3: Navigate to /en/checkout ────────────────────────────────────
    runtime.log("--- Step 3: navigate to /en/checkout ---");
    await d.get(EN + "/checkout");
    await runtime.time.sleep(1500);
    runtime.log("checkout title:", await d.title(), "url:", await d.url());

    const pageText: string = await d.executeScript(
      `return document.body ? document.body.innerText : ""`, []);
    const basketEmpty = /basket is empty|kundvagnen är tom|tom korg/i.test(pageText);
    if (basketEmpty) {
      runtime.log(
        "CART STATE: empty at /en/checkout.",
        "This is expected if the session is not authenticated OR if the login did not succeed.",
        "Real end-to-end checkout requires a working authenticated session. See notes in script header."
      );
    } else {
      runtime.log("CART STATE: checkout page shows cart content.");
    }

    // ── Step 4: Locate provider UI (dry-run — do not enter payment) ─────────
    runtime.log("--- Step 4: locate provider UI (dry-run) ---");
    const foundContainer: boolean = await d.executeScript(`
      const sel = ${JSON.stringify(prov.optionSelector)};
      const sels = sel.split(',').map(s => s.trim());
      for (const s of sels) {
        const el = document.querySelector(s);
        if (el) return true;
      }
      return false;
    `, []);

    const foundIframe: boolean = await d.executeScript(`
      const sel = ${JSON.stringify(prov.iframeSelector)};
      const sels = sel.split(',').map(s => s.trim());
      for (const s of sels) {
        const el = document.querySelector(s);
        if (el) return true;
      }
      return false;
    `, []);

    runtime.log("provider container found:", foundContainer, "/ iframe found:", foundIframe);

    // ── DRY-RUN STOP ────────────────────────────────────────────────────────
    runtime.log(`DRY-RUN STOP: would proceed to enter payment for ${prov.label}.`);
    runtime.log("Payment test data:", prov.notes);
    runtime.log(
      "Full payment automation (iframe navigation, 3DS, ngrok 'Visit Site') is best driven by",
      "the Playwright /devshop skill, e.g.: /devshop buy watch-arne-jacobsen via " + provider
    );

  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
