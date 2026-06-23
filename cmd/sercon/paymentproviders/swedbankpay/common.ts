import { apiRequest, ClientCtx } from "../core/http";
import { envGet, pickBaseUrl } from "../core/config";
import { findOperation } from "../core/hal";

const TEST_URL = "https://api.externalintegration.payex.com";
const PROD_URL = "https://api.payex.com";

export interface SwedbankPayConfig {
  accessToken?: string;
  merchantId?: string;
  env?: "test" | "prod";
  baseUrl?: string;
}

export interface SwedbankPayClient {
  merchantId: string;
  createPaymentOrder(body: Record<string, unknown>): Promise<any>;
  getPaymentOrder(idOrUrl: string): Promise<any>;
  getPayment(idOrUrl: string): Promise<any>;
  operation(paymentOrderOrUrl: string | Record<string, unknown>, rel: string, body?: unknown): Promise<any>;
  capturePayment(paymentOrderOrUrl: string | Record<string, unknown>, body: Record<string, unknown>): Promise<any>;
  refundPayment(paymentOrderOrUrl: string | Record<string, unknown>, body: Record<string, unknown>): Promise<any>;
  cancelPayment(paymentOrderOrUrl: string | Record<string, unknown>, body?: Record<string, unknown>): Promise<any>;
}

export interface VersionConfig { version: string; createPath: string; }

// buildClient is the shared SwedbankPay client used by both version namespaces.
export function buildClient(overrides: SwedbankPayConfig, ver: VersionConfig): SwedbankPayClient {
  const accessToken = overrides.accessToken ?? envGet("SWEDBANKPAY_ACCESS_TOKEN");
  const merchantId = overrides.merchantId ?? envGet("SWEDBANKPAY_MERCHANT_ID");
  if (!accessToken) throw new Error(`${ver.version}: SWEDBANKPAY_ACCESS_TOKEN is required (set it in the environment/.env or pass accessToken)`);
  if (!merchantId) throw new Error(`${ver.version}: SWEDBANKPAY_MERCHANT_ID is required (set it in the environment/.env or pass merchantId)`);
  const env = overrides.env ?? (envGet("SWEDBANKPAY_ENV") as "test" | "prod" | undefined);
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("SWEDBANKPAY_BASE_URL"), TEST_URL, PROD_URL);
  const ctx: ClientCtx = { baseUrl, provider: ver.version, sign: () => ({ Authorization: "Bearer " + accessToken }) };

  const getPaymentOrder = (idOrUrl: string) => apiRequest("GET", idOrUrl, ctx);

  // operation resolves a HAL operation by rel and POSTs to its (absolute) href.
  // Accepts an already-fetched payment-order payload, or an id/url to GET first.
  const operation = async (paymentOrderOrUrl: string | Record<string, unknown>, rel: string, body?: unknown) => {
    const po = typeof paymentOrderOrUrl === "string" ? await getPaymentOrder(paymentOrderOrUrl) : paymentOrderOrUrl;
    const op = findOperation(po, rel);
    if (!op) throw new Error(`${ver.version}: operation '${rel}' is not available on this payment order`);
    return apiRequest(op.method, op.href, ctx, body);
  };

  return {
    merchantId,
    createPaymentOrder: (body) => apiRequest("POST", ver.createPath, ctx, body),
    getPaymentOrder,
    getPayment: getPaymentOrder,
    operation,
    // rel names are SEAMs — confirmed live.
    capturePayment: (poOrUrl, body) => operation(poOrUrl, "capture", body),
    refundPayment: (poOrUrl, body) => operation(poOrUrl, "reversal", body),
    cancelPayment: (poOrUrl, body) => operation(poOrUrl, "cancel", body),
  };
}
