// Shared helper module for sws6 WebDriver test scripts.
// Exports: BASE, EN, persona, loginCreds, shopUp().
// Not a runnable demo — import from the other sws6 scripts.
// Self-safe: no top-level side effects; exits 0 when run directly.

/** Root of the dev shop (plain HTTP). */
export const BASE = "http://dev-shop.sws.local";

/** English locale root. */
export const EN = BASE + "/en";

/** Fictional test persona for checkout / registration flows. */
export const persona = {
  firstName: "Tess",
  lastName:  "T Person",
  address:   "Testvägen 1",
  zip:       "12345",
  city:      "Testinge",
  country:   "Sweden",
  email:     "***REMOVED***",
  phone:     "+46 700 00 00 00",
};

/** Credentials for the existing test-customer account. */
export const loginCreds = {
  email:    "***REMOVED***",
  password: "***REMOVED***",
};

/**
 * Returns true if the dev shop's English home page is reachable,
 * false if the HTTP request throws (host down / DNS fail).
 */
export async function shopUp(): Promise<boolean> {
  try {
    await net.http.get(EN);
    return true;
  } catch {
    return false;
  }
}
