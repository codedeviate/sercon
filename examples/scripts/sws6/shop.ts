// Shared helper module for the sws6 dev-shop WebDriver test scripts.
// Not a runnable demo — imported by the other sws6 scripts. No top-level side
// effects, so running it directly just exits 0.
//
// Secrets (login credentials, payment test data) are read from the
// ENVIRONMENT, never hard-coded here — so they don't live in git. Copy
// `sws6/.env.example` to `sws6/.env` (gitignored), fill it in, and load it
// before running, e.g.:
//
//   set -a; source examples/scripts/sws6/.env; set +a
//   ./sercon examples/scripts/sws6/login.ts
//
// Anything not set falls back to a safe non-secret default (or empty, which
// makes the dependent script self-skip with a helpful message).

function env(name: string, fallback = ""): string {
  return runtime.env.get(name) ?? fallback;
}

/** Root of the dev shop. Override with DEVSHOP_BASE; defaults to the dev host. */
export const BASE = env("DEVSHOP_BASE", "http://dev-shop.sws.local");

/** English locale root. */
export const EN = BASE + "/en";

/** DOM signal that a customer is authenticated (per the shop's markup). */
export const LOGOUT_SEL = 'a[href*="/customer/logout"]';

/** A realistic desktop Chrome user-agent. The dev shop withholds its session
 *  cookie (`swssid`) from the default `HeadlessChrome` UA, so WITHOUT this the
 *  session never establishes and the cart/login don't persist across requests.
 *  Override with DEVSHOP_UA if needed. */
export const USER_AGENT = env(
  "DEVSHOP_UA",
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
    "(KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36",
);

/** Connect a headless Chrome WebDriver session configured for the dev shop —
 *  notably with a non-headless UA so the `swssid` session cookie is issued. */
export async function connectShop() {
  return await services.webdriver.connect({
    browser: "chrome",
    headless: true,
    args: ["--user-agent=" + USER_AGENT],
  });
}

/** Credentials for the existing test-customer account (from the environment). */
export const loginCreds = {
  email: env("DEVSHOP_EMAIL"),
  password: env("DEVSHOP_PASSWORD"),
};

/** True when both login credentials are present in the environment. */
export function haveCreds(): boolean {
  return !!(loginCreds.email && loginCreds.password);
}

/** Fictional buyer persona. Names/address are obvious placeholders; the
 *  contact fields come from the environment so a real address/email isn't
 *  committed. */
export const persona = {
  firstName: "Tess",
  lastName: "T Person",
  address: "Testvägen 1",
  zip: "12345",
  city: "Testinge",
  country: "Sweden",
  email: env("DEVSHOP_PERSONA_EMAIL", "tess.person@example.com"),
  phone: env("DEVSHOP_PERSONA_PHONE", "+46 700 00 00 00"),
};

/**
 * Per-provider payment test data, read from the environment. Returns whatever
 * is set (fields may be empty). Public provider test values still live in the
 * gitignored .env rather than in the committed script. Codes: kco|sco|nets|qliro.
 */
export function paymentData(code: string): Record<string, string> {
  const c = code.toLowerCase();
  const card = env("DEVSHOP_CARD"); // default test card
  const exp = env("DEVSHOP_CARD_EXP");
  const ccv = env("DEVSHOP_CARD_CCV");
  switch (c) {
    case "sco":
      return { card: env("DEVSHOP_SCO_CARD"), exp, ccv, personnr: env("DEVSHOP_SCO_PERSONNR"), orgnr: env("DEVSHOP_SCO_ORGNR"), postal: env("DEVSHOP_SCO_POSTAL") };
    case "nets":
      return { card: env("DEVSHOP_NETS_CARD"), exp, ccv, personnr: env("DEVSHOP_NETS_PERSONNR"), postal: env("DEVSHOP_NETS_POSTAL") };
    case "qliro":
      return { card, exp, ccv, personnr: env("DEVSHOP_QLIRO_PERSONNR"), orgnr: env("DEVSHOP_QLIRO_ORGNR") };
    default: // kco — default persona card
      return { card, exp, ccv };
  }
}

/** Returns true if the dev shop's English home page is reachable. */
export async function shopUp(): Promise<boolean> {
  try {
    await net.http.get(EN);
    return true;
  } catch {
    return false;
  }
}
