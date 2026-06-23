declare const net: any;
import { PaymentError } from "./errors";

export interface ClientCtx {
  baseUrl: string;
  provider: string;
  // sign receives the method, path, and serialized body (providers like
  // Svea/Qliro sign over the body) and returns auth headers to merge.
  sign: (method: string, path: string, bodyStr: string) => Record<string, string>;
}

// idempotencyKey returns a unique key for a mutating request (overridable).
export function idempotencyKey(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

// apiRequest performs a JSON request via net.http (with the ctx signer's auth
// headers) and throws PaymentError on a non-2xx response.
export async function apiRequest(
  method: string,
  path: string,
  ctx: ClientCtx,
  body?: unknown,
  extraHeaders?: Record<string, string>,
): Promise<any> {
  const headers: Record<string, string> = { accept: "application/json", ...(extraHeaders || {}) };
  let bodyStr = "";
  if (body !== undefined) {
    headers["content-type"] = "application/json";
    bodyStr = JSON.stringify(body);
  }
  const authHeaders = ctx.sign(method, path, bodyStr);
  for (const k in authHeaders) headers[k] = authHeaders[k];
  const res = await net.http.request(method, ctx.baseUrl + path, {
    headers,
    body: bodyStr,
    follow: true,
  });
  let parsed: unknown;
  if (res.body) {
    try { parsed = JSON.parse(res.body); } catch { parsed = res.body; }
  }
  if (!(res.status >= 200 && res.status < 300)) {
    const reqId = res.headers && (res.headers["x-correlation-id"] || res.headers["klarna-correlation-id"]);
    throw new PaymentError(ctx.provider, res.status, parsed, reqId);
  }
  return parsed;
}
