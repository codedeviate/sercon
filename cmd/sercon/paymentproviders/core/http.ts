declare const net: any;
import { PaymentError } from "./errors";

export interface ClientCtx {
  baseUrl: string;
  provider: string;
  username: string;
  password: string;
}

// idempotencyKey returns a unique key for a mutating request (overridable).
export function idempotencyKey(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

// apiRequest performs a JSON request via net.http and throws PaymentError on a
// non-2xx response. body (when given) is JSON-encoded; extraHeaders are merged.
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
  const res = await net.http.request(method, ctx.baseUrl + path, {
    headers,
    body: bodyStr,
    username: ctx.username,
    password: ctx.password,
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
