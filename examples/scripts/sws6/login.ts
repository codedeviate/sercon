// sws6/login.ts — Logs in to dev-shop.sws.local with a test customer account.
//
// Steps:
//   1. Navigate to /en/customer
//   2. Click #existing-account-type-radio
//   3. Fill #login-email-field + #login-password-field
//   4. Click button.login-action
//   5. Observe and report post-submit state
//
// NOTE: After submitting the login form, the script reports the observed DOM
// state (URL, whether the form field disappeared, any visible error text).
// It does NOT hard-assert "logged in" — the shop's logged-in UI indicators
// were not reliably observable during development (no "log out" link / account
// name was confirmed). Confirm your own authenticated-state signal from the
// shop's HTML and tighten this assertion for your environment.
//
// Exit: always 0 (no throw on the ambiguous post-login state).

import { EN, loginCreds, shopUp } from "./shop.ts";

if (!services.webdriver.available) {
  runtime.log("no chromedriver on PATH — skipping login demo.");
} else if (!(await shopUp())) {
  runtime.log("dev-shop.sws.local unreachable — skipping login demo.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // ── Step 1: open the customer/login page ────────────────────────────────
    await d.get(EN + "/customer");
    runtime.log("page:", await d.title(), "url:", await d.url());

    // ── Step 2: select the "existing account" radio ─────────────────────────
    try {
      const radio = await d.find("css", "#existing-account-type-radio");
      await radio.click();
      runtime.log("clicked #existing-account-type-radio");
    } catch (e) {
      runtime.log("WARN: could not click #existing-account-type-radio:", String(e));
    }

    // ── Step 3: fill credentials ─────────────────────────────────────────────
    // Give the UI a moment to reveal the login fields after clicking the radio.
    await runtime.time.sleep(800);
    const emailField = await d.find("css", "#login-email-field");
    await emailField.sendKeys(loginCreds.email);
    runtime.log("filled email:", loginCreds.email);

    const passwordField = await d.find("css", "#login-password-field");
    await passwordField.sendKeys(loginCreds.password);
    runtime.log("filled password: ***");

    // ── Step 4: submit ───────────────────────────────────────────────────────
    const loginBtn = await d.find("css", "button.login-action");
    await loginBtn.click();
    runtime.log("clicked button.login-action");

    // Give the page a moment to react.
    await runtime.time.sleep(2000);

    // ── Step 5: observe post-submit state ───────────────────────────────────
    const afterUrl = await d.url();
    runtime.log("post-submit url:", afterUrl);

    // Is the password field still in the DOM? (disappears on successful submit)
    let formGone = false;
    try {
      await d.find("css", "#login-password-field");
      runtime.log("post-submit: #login-password-field still present (login may have failed)");
    } catch {
      formGone = true;
      runtime.log("post-submit: #login-password-field gone (form submitted)");
    }

    // Look for any visible error text.
    let errorText = "";
    try {
      const errEl = await d.find("css", ".login-error, .error-message, [class*=error]");
      errorText = (await errEl.text()).trim();
    } catch {
      // no error element found
    }

    if (errorText) {
      runtime.log("WARN: visible error text after submit:", errorText);
    } else {
      runtime.log("no visible error text found after submit.");
    }

    // Assert: no error message was shown. This is the only reliable signal
    // we can confirm — it does not positively confirm authentication.
    runtime.assert.equal(errorText, "", "expected no login error text after submit");

    if (formGone) {
      runtime.log("LOGIN LIKELY SUCCEEDED (form gone, no error). " +
        "Authenticated-state confirmation depends on the shop's logged-in UI — " +
        "add a shop-specific selector check (e.g. 'a[href*=logout]') to verify.");
    } else {
      runtime.log("LOGIN STATUS UNCLEAR (form still present). " +
        "Check credentials or the shop's current behaviour.");
    }
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
