import { apiRequest, idempotencyKey, ClientCtx } from "../core/http";
import { envGet, pickBaseUrl } from "../core/config";

const TEST_URL = "https://api.playground.kustom.co";
const PROD_URL = "https://api.kustom.co";

export interface KcoConfig {
  merchantId?: string;
  sharedSecret?: string;
  env?: "test" | "prod";
  baseUrl?: string;
}
export interface CaptureInput { amount: number; orderLines?: unknown[]; description?: string; }
export interface RefundInput { amount: number; orderLines?: unknown[]; description?: string; }

export interface KcoClient {
  getPayment(id: string): Promise<any>;
  acknowledge(id: string): Promise<any>;
  capturePayment(id: string, input: CaptureInput): Promise<any>;
  refundPayment(id: string, input: RefundInput): Promise<any>;
  cancelPayment(id: string): Promise<any>;
  releaseRemainingAuthorization(id: string): Promise<any>;
  createCheckout(order: Record<string, unknown>): Promise<any>;
  getCheckout(id: string): Promise<any>;
}

export function client(overrides: KcoConfig = {}): KcoClient {
  const merchantId = overrides.merchantId ?? envGet("KCO_MERCHANT_ID");
  const sharedSecret = overrides.sharedSecret ?? envGet("KCO_SHARED_SECRET");
  if (!merchantId) throw new Error("kcov3: KCO_MERCHANT_ID is required (set it in the environment/.env or pass merchantId)");
  if (!sharedSecret) throw new Error("kcov3: KCO_SHARED_SECRET is required (set it in the environment/.env or pass sharedSecret)");
  const env = overrides.env ?? (envGet("KCO_ENV") as "test" | "prod" | undefined);
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("KCO_BASE_URL"), TEST_URL, PROD_URL);
  const ctx: ClientCtx = { baseUrl, provider: "kcov3", username: merchantId, password: sharedSecret };

  const om = (id: string) => `/ordermanagement/v1/orders/${encodeURIComponent(id)}`;
  const idem = () => ({ "klarna-idempotency-key": idempotencyKey() });

  return {
    getPayment: (id) => apiRequest("GET", om(id), ctx),
    acknowledge: (id) => apiRequest("POST", `${om(id)}/acknowledge`, ctx, undefined, idem()),
    capturePayment: (id, input) =>
      apiRequest("POST", `${om(id)}/captures`, ctx,
        { captured_amount: input.amount, order_lines: input.orderLines, description: input.description }, idem()),
    refundPayment: (id, input) =>
      apiRequest("POST", `${om(id)}/refunds`, ctx,
        { refunded_amount: input.amount, order_lines: input.orderLines, description: input.description }, idem()),
    cancelPayment: (id) => apiRequest("POST", `${om(id)}/cancel`, ctx, undefined, idem()),
    releaseRemainingAuthorization: (id) =>
      apiRequest("POST", `${om(id)}/release-remaining-authorization`, ctx, undefined, idem()),
    createCheckout: (order) => apiRequest("POST", "/checkout/v3/orders", ctx, order),
    getCheckout: (id) => apiRequest("GET", `/checkout/v3/orders/${encodeURIComponent(id)}`, ctx),
  };
}
