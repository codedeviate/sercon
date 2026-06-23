import { apiRequest, ClientCtx } from "../core/http";
import { envGet, pickBaseUrl } from "../core/config";

// SEAM: base URLs to confirm against the live test creds.
const TEST_URL = "https://test.api.dibspayment.eu";
const PROD_URL = "https://api.dibspayment.eu";

export interface NetsConfig { secretKey?: string; env?: "test" | "prod"; baseUrl?: string; }
export interface NetsAmountInput { amount: number; orderItems?: unknown[]; }

export interface NetsClient {
  createPayment(payment: Record<string, unknown>): Promise<any>;
  getPayment(id: string): Promise<any>;
  capturePayment(id: string, input: NetsAmountInput): Promise<any>;
  refundPayment(id: string, input: NetsAmountInput): Promise<any>;
  cancelPayment(id: string): Promise<any>;
}

export function client(overrides: NetsConfig = {}): NetsClient {
  const secretKey = overrides.secretKey ?? envGet("NETS_SECRET_KEY");
  if (!secretKey) throw new Error("netsv1: NETS_SECRET_KEY is required (set it in the environment/.env or pass secretKey)");
  const env = overrides.env ?? (envGet("NETS_ENV") as "test" | "prod" | undefined);
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("NETS_BASE_URL"), TEST_URL, PROD_URL);
  // Nexi/Nets: the secret key is the Authorization header value (no "Bearer").
  const ctx: ClientCtx = { baseUrl, provider: "netsv1", sign: () => ({ Authorization: secretKey }) };

  const p = (id: string) => `/v1/payments/${encodeURIComponent(id)}`;
  return {
    createPayment: (payment) => apiRequest("POST", "/v1/payments", ctx, payment),
    getPayment: (id) => apiRequest("GET", p(id), ctx),
    // SEAM: charge/refund body shape ({ amount, orderItems }) — confirm against docs/live.
    capturePayment: (id, input) => apiRequest("POST", `${p(id)}/charges`, ctx, { amount: input.amount, orderItems: input.orderItems }),
    refundPayment: (id, input) => apiRequest("POST", `${p(id)}/refunds`, ctx, { amount: input.amount, orderItems: input.orderItems }),
    cancelPayment: (id) => apiRequest("PUT", `${p(id)}/terminate`, ctx),
  };
}
