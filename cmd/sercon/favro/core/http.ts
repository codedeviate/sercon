declare const net: any;
import { FavroError, RateLimitInfo } from "./errors";

export interface RetryConfig { max: number; maxWaitMs: number; }

export interface ClientCtx {
  baseUrl: string;
  authHeader: string;          // "Basic ...."
  organizationId?: string;
  retry: RetryConfig | false;
}

export interface FavroResponse {
  status: number;
  headers: Record<string, string>;   // lower-cased single-value (from net.http)
  body: any;                          // parsed JSON (or raw text)
}

export interface RequestOpts {
  query?: Record<string, string | number | boolean | undefined>;
  body?: unknown;
  orgScoped?: boolean;                // default true
  headers?: Record<string, string>;
}

function num(h: Record<string, string>, k: string): number | undefined {
  const v = h[k];
  if (v === undefined) return undefined;
  const n = Number(v);
  return isNaN(n) ? undefined : n;
}

export function rateLimitOf(h: Record<string, string>): RateLimitInfo {
  return {
    limit: num(h, "x-ratelimit-limit"),
    remaining: num(h, "x-ratelimit-remaining"),
    reset: h["x-ratelimit-reset"],
    retryAfter: num(h, "retry-after"),
  };
}

export function buildUrl(base: string, path: string, query?: RequestOpts["query"]): string {
  let url = path.startsWith("http") ? path : base + path;
  if (query) {
    const parts: string[] = [];
    for (const k in query) {
      const v = query[k];
      if (v !== undefined) parts.push(encodeURIComponent(k) + "=" + encodeURIComponent(String(v)));
    }
    if (parts.length) url += (url.includes("?") ? "&" : "?") + parts.join("&");
  }
  return url;
}

// request performs one JSON API call and returns status + headers + parsed
// body. Throws FavroError on non-2xx. Task 2 adds 429 retry to this function.
export async function request(ctx: ClientCtx, method: string, path: string, opts: RequestOpts = {}): Promise<FavroResponse> {
  const orgScoped = opts.orgScoped !== false;
  const url = buildUrl(ctx.baseUrl, path, opts.query);
  const headers: Record<string, string> = { accept: "application/json", authorization: ctx.authHeader, ...(opts.headers || {}) };
  if (orgScoped && ctx.organizationId) headers["organizationId"] = ctx.organizationId;
  let bodyStr: string | undefined;
  if (opts.body !== undefined) {
    headers["content-type"] = "application/json";
    bodyStr = JSON.stringify(opts.body);
  }

  const res = await net.http.request(method, url, { headers, body: bodyStr, follow: true });
  const respHeaders: Record<string, string> = res.headers || {};
  let parsed: unknown = undefined;
  if (res.body) {
    try { parsed = JSON.parse(res.body); } catch { parsed = res.body; }
  }
  if (res.status >= 200 && res.status < 300) {
    return { status: res.status, headers: respHeaders, body: parsed };
  }
  const requestId = typeof (parsed as any)?.requestId === "string" ? (parsed as any).requestId : undefined;
  throw new FavroError(res.status, parsed, requestId, rateLimitOf(respHeaders));
}
