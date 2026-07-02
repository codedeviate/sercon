declare const runtime: any;

export function envGet(name: string): string | undefined {
  const v = runtime.env.get(name);
  return v === undefined || v === null ? undefined : String(v);
}

export const DEFAULT_BASE_URL = "https://favro.com/api/v1";

// resolveBaseUrl: explicit baseUrl wins, else FAVRO_BASE_URL, else the default.
// Trailing slashes trimmed so callers concatenate paths cleanly. Favro has a
// single base URL (no test/prod split).
export function resolveBaseUrl(baseUrl: string | undefined): string {
  const b = baseUrl ?? envGet("FAVRO_BASE_URL") ?? DEFAULT_BASE_URL;
  return b.replace(/\/+$/, "");
}
