declare const runtime: any;

export function envGet(name: string): string | undefined {
  const v = runtime.env.get(name);
  return v === undefined || v === null ? undefined : String(v);
}

// Choose a base URL: explicit baseUrl wins; otherwise env selects test/prod.
// Trailing slashes are trimmed so callers can concatenate paths.
export function pickBaseUrl(
  env: string | undefined,
  baseUrl: string | undefined,
  testUrl: string,
  prodUrl: string,
): string {
  if (baseUrl) return baseUrl.replace(/\/+$/, "");
  return env === "prod" ? prodUrl : testUrl;
}
