import { apiRequest, ClientCtx } from "../core/http";
import { envGet, pickBaseUrl } from "../core/config";
import { sha256Base64 } from "../core/crypto";

// SEAM: base URLs to confirm against the live test creds.
const TEST_URL = "https://pago.qit.nu";
const PROD_URL = "https://payments.qliro.com";

export interface QliroConfig { apiKey?: string; apiPassword?: string; env?: "test" | "prod"; baseUrl?: string; }

export interface QliroClient {
  createOrder(order: Record<string, unknown>): Promise<any>;
  getOrder(id: string | number): Promise<any>;
  getPayment(id: string | number): Promise<any>;
}

export function client(overrides: QliroConfig = {}): QliroClient {
  const apiKey = overrides.apiKey ?? envGet("QLIRO_API_KEY");
  const apiPassword = overrides.apiPassword ?? envGet("QLIRO_APIPASSWORD");
  if (!apiKey) throw new Error("qlirov2: QLIRO_API_KEY is required (set it in the environment/.env or pass apiKey)");
  if (!apiPassword) throw new Error("qlirov2: QLIRO_APIPASSWORD is required (set it in the environment/.env or pass apiPassword)");
  const env = overrides.env ?? (envGet("QLIRO_ENV") as "test" | "prod" | undefined);
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("QLIRO_BASE_URL"), TEST_URL, PROD_URL);

  // Auth (pinned): token = base64(SHA256(bodyStr + apiSecret)); header `Authorization: Qliro <token>`.
  const sign = (_m: string, _p: string, bodyStr: string) => ({ Authorization: "Qliro " + sha256Base64(bodyStr + apiPassword) });
  const ctx: ClientCtx = { baseUrl, provider: "qlirov2", sign };

  // SEAM: Qliro's merchant API is RPC-style; the API key rides in the body
  // (MerchantApiKey). Paths + key placement confirmed live later.
  return {
    createOrder: (order) => apiRequest("POST", "/checkout/merchantapi/Orders", ctx, { MerchantApiKey: apiKey, ...order }),
    getOrder: (id) => apiRequest("POST", "/checkout/merchantapi/Orders/GetOrder", ctx, { MerchantApiKey: apiKey, OrderId: id }),
    getPayment: (id) => apiRequest("POST", "/checkout/merchantapi/Orders/GetOrder", ctx, { MerchantApiKey: apiKey, OrderId: id }),
  };
}
