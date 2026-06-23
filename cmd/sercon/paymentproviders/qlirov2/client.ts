import { apiRequest, ClientCtx } from "../core/http";
import { envGet, pickBaseUrl } from "../core/config";
import { sha256Base64 } from "../core/crypto";

// Base URLs pinned from the reference integration: pago.qit.nu (test) /
// payments.qit.nu (live).
const TEST_URL = "https://pago.qit.nu";
const PROD_URL = "https://payments.qit.nu";

export interface QliroConfig { apiKey?: string; apiPassword?: string; env?: "test" | "prod"; baseUrl?: string; }

export interface QliroClient {
  createOrder(order: Record<string, unknown>): Promise<any>;
  getOrder(id: string | number): Promise<any>;
  getPayment(id: string | number): Promise<any>;
  capturePayment(orderId: string | number, input?: Record<string, unknown>): Promise<any>;
  refundPayment(orderId: string | number, input?: Record<string, unknown>): Promise<any>;
  cancelPayment(orderId: string | number): Promise<any>;
}

// uuidV4 returns an RFC4122-v4-shaped id for the management RequestId field.
function uuidV4(): string {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

export function client(overrides: QliroConfig = {}): QliroClient {
  const apiKey = overrides.apiKey ?? envGet("QLIRO_API_KEY");
  const apiPassword = overrides.apiPassword ?? envGet("QLIRO_APIPASSWORD");
  if (!apiKey) throw new Error("qlirov2: QLIRO_API_KEY is required (set it in the environment/.env or pass apiKey)");
  if (!apiPassword) throw new Error("qlirov2: QLIRO_APIPASSWORD is required (set it in the environment/.env or pass apiPassword)");
  const env = overrides.env ?? (envGet("QLIRO_ENV") as "test" | "prod" | undefined);
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("QLIRO_BASE_URL"), TEST_URL, PROD_URL);

  // Auth: token = base64(SHA256(bodyStr + apiSecret)); header `Authorization: Qliro <token>`.
  // For a bodyless GET, bodyStr is "" so the token signs just the secret — which is
  // exactly Qliro's scheme for GETs.
  const sign = (_m: string, _p: string, bodyStr: string) => ({ Authorization: "Qliro " + sha256Base64(bodyStr + apiPassword) });
  const ctx: ClientCtx = { baseUrl, provider: "qlirov2", sign };

  // Management (adminapi/v2) calls carry MerchantApiKey + a RequestId + OrderId in
  // the signed body. MerchantApiKey is injected last so it is authoritative.
  const mgmt = (orderId: string | number, extra: Record<string, unknown>) =>
    ({ RequestId: uuidV4(), OrderId: orderId, ...extra, MerchantApiKey: apiKey });

  const oid = (id: string | number) => `/checkout/merchantapi/Orders/${encodeURIComponent(String(id))}`;
  return {
    // createOrder: MerchantApiKey injected last so a caller's order cannot override it.
    createOrder: (order) => apiRequest("POST", "/checkout/merchantapi/Orders", ctx, { ...order, MerchantApiKey: apiKey }),
    // getOrder is RESTful: GET /Orders/{id}, no body. getPayment is an alias (the order IS the payment).
    getOrder: (id) => apiRequest("GET", oid(id), ctx),
    getPayment: (id) => apiRequest("GET", oid(id), ctx),
    // Admin API v2 management: capture = MarkItemsAsShipped, refund = ReturnItems, cancel = cancelOrder.
    capturePayment: (orderId, input = {}) => apiRequest("POST", "/checkout/adminapi/v2/MarkItemsAsShipped", ctx, mgmt(orderId, input)),
    refundPayment: (orderId, input = {}) => apiRequest("POST", "/checkout/adminapi/v2/ReturnItems", ctx, mgmt(orderId, input)),
    cancelPayment: (orderId) => apiRequest("POST", "/checkout/adminapi/v2/cancelOrder", ctx, mgmt(orderId, {})),
  };
}
