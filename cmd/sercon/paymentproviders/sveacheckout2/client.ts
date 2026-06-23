import { apiRequest, ClientCtx } from "../core/http";
import { envGet, pickBaseUrl } from "../core/config";
import { sha512HexUpper } from "../core/crypto";

declare const text: any;

const TEST_URL = "https://checkoutapistage.svea.com";
const PROD_URL = "https://checkoutapi.svea.com"; // SEAM: confirm live

export interface SveaConfig { merchantId?: string; secretKey?: string; env?: "test" | "prod"; baseUrl?: string; }

export interface SveaClient {
  createOrder(order: Record<string, unknown>): Promise<any>;
  getOrder(id: string): Promise<any>;
  getPayment(id: string): Promise<any>;
  capturePayment(id: string, input: { amount: number; rows?: unknown[] }): Promise<any>;
  refundPayment(id: string, input: { amount: number; rows?: unknown[] }): Promise<any>;
  cancelPayment(id: string): Promise<any>;
}

// Svea timestamp: UTC "YYYY-MM-DD HH:MM:SS".
function sveaTimestamp(): string {
  return new Date().toISOString().replace("T", " ").split(".")[0];
}

export function client(overrides: SveaConfig = {}): SveaClient {
  const merchantId = overrides.merchantId ?? envGet("SCO_MERCHANT_ID");
  const secretKey = overrides.secretKey ?? envGet("SCO_SECRET_KEY");
  if (!merchantId) throw new Error("sveacheckout2: SCO_MERCHANT_ID is required (set it in the environment/.env or pass merchantId)");
  if (!secretKey) throw new Error("sveacheckout2: SCO_SECRET_KEY is required (set it in the environment/.env or pass secretKey)");
  const env = overrides.env ?? (envGet("SCO_ENV") as "test" | "prod" | undefined);
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("SCO_BASE_URL"), TEST_URL, PROD_URL);

  // Auth (pinned): token = base64(merchantId:UPPER(sha512(body+secret+ts))),
  // sent as `Authorization: Svea <token>` + a `Timestamp` header. bodyStr "" for GETs.
  const sign = (_m: string, _p: string, bodyStr: string) => {
    const ts = sveaTimestamp();
    const hash = sha512HexUpper(bodyStr + secretKey + ts);
    const token = text.str.base64Encode(`${merchantId}:${hash}`);
    return { Authorization: "Svea " + token, Timestamp: ts }; // SEAM: confirm header name "Timestamp"
  };
  const ctx: ClientCtx = { baseUrl, provider: "sveacheckout2", sign };

  const ord = (id: string) => `/api/orders/${encodeURIComponent(id)}`;
  return {
    createOrder: (order) => apiRequest("POST", "/api/orders", ctx, order),
    getOrder: (id) => apiRequest("GET", ord(id), ctx),
    getPayment: (id) => apiRequest("GET", ord(id), ctx),
    // SEAM paths below — confirmed/fixed live later.
    capturePayment: (id, input) => apiRequest("POST", `${ord(id)}/deliveries`, ctx, { amount: input.amount, rows: input.rows }),
    refundPayment: (id, input) => apiRequest("POST", `${ord(id)}/credits`, ctx, { amount: input.amount, rows: input.rows }),
    cancelPayment: (id) => apiRequest("POST", `${ord(id)}/cancel`, ctx),
  };
}
