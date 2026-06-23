var __defProp = Object.defineProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};

// cmd/sercon/paymentproviders/kcov3/client.ts
var client_exports = {};
__export(client_exports, {
  client: () => client
});

// cmd/sercon/paymentproviders/core/errors.ts
var PaymentError = class extends Error {
  provider;
  status;
  body;
  requestId;
  constructor(provider, status, body, requestId) {
    super(`${provider}: HTTP ${status}`);
    this.name = "PaymentError";
    this.provider = provider;
    this.status = status;
    this.body = body;
    this.requestId = requestId;
  }
};

// cmd/sercon/paymentproviders/core/http.ts
function idempotencyKey() {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}
async function apiRequest(method, path, ctx, body, extraHeaders) {
  const headers = { accept: "application/json", ...extraHeaders || {} };
  let bodyStr = "";
  if (body !== void 0) {
    headers["content-type"] = "application/json";
    bodyStr = JSON.stringify(body);
  }
  const authHeaders = ctx.sign(method, path, bodyStr);
  for (const k in authHeaders) headers[k] = authHeaders[k];
  const res = await net.http.request(method, ctx.baseUrl + path, {
    headers,
    body: bodyStr,
    follow: true
  });
  let parsed;
  if (res.body) {
    try {
      parsed = JSON.parse(res.body);
    } catch {
      parsed = res.body;
    }
  }
  if (!(res.status >= 200 && res.status < 300)) {
    const reqId = res.headers && (res.headers["x-correlation-id"] || res.headers["klarna-correlation-id"]);
    throw new PaymentError(ctx.provider, res.status, parsed, reqId);
  }
  return parsed;
}

// cmd/sercon/paymentproviders/core/config.ts
function envGet(name) {
  const v = runtime.env.get(name);
  return v === void 0 || v === null ? void 0 : String(v);
}
function pickBaseUrl(env, baseUrl, testUrl, prodUrl) {
  if (baseUrl) return baseUrl.replace(/\/+$/, "");
  return env === "prod" ? prodUrl : testUrl;
}

// cmd/sercon/paymentproviders/core/crypto.ts
function basicAuth(user, pass) {
  return "Basic " + text.str.base64Encode(`${user}:${pass}`);
}

// cmd/sercon/paymentproviders/kcov3/client.ts
var TEST_URL = "https://api.playground.kustom.co";
var PROD_URL = "https://api.kustom.co";
function client(overrides = {}) {
  const merchantId = overrides.merchantId ?? envGet("KCO_MERCHANT_ID");
  const sharedSecret = overrides.sharedSecret ?? envGet("KCO_SHARED_SECRET");
  if (!merchantId) throw new Error("kcov3: KCO_MERCHANT_ID is required (set it in the environment/.env or pass merchantId)");
  if (!sharedSecret) throw new Error("kcov3: KCO_SHARED_SECRET is required (set it in the environment/.env or pass sharedSecret)");
  const env = overrides.env ?? envGet("KCO_ENV");
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("KCO_BASE_URL"), TEST_URL, PROD_URL);
  const ctx = { baseUrl, provider: "kcov3", sign: () => ({ Authorization: basicAuth(merchantId, sharedSecret) }) };
  const om = (id) => `/ordermanagement/v1/orders/${encodeURIComponent(id)}`;
  const idem = () => ({ "klarna-idempotency-key": idempotencyKey() });
  return {
    getPayment: (id) => apiRequest("GET", om(id), ctx),
    acknowledge: (id) => apiRequest("POST", `${om(id)}/acknowledge`, ctx, void 0, idem()),
    capturePayment: (id, input) => apiRequest(
      "POST",
      `${om(id)}/captures`,
      ctx,
      { captured_amount: input.amount, order_lines: input.orderLines, description: input.description },
      idem()
    ),
    refundPayment: (id, input) => apiRequest(
      "POST",
      `${om(id)}/refunds`,
      ctx,
      { refunded_amount: input.amount, order_lines: input.orderLines, description: input.description },
      idem()
    ),
    cancelPayment: (id) => apiRequest("POST", `${om(id)}/cancel`, ctx, void 0, idem()),
    releaseRemainingAuthorization: (id) => apiRequest("POST", `${om(id)}/release-remaining-authorization`, ctx, void 0, idem()),
    createCheckout: (order) => apiRequest("POST", "/checkout/v3/orders", ctx, order),
    getCheckout: (id) => apiRequest("GET", `/checkout/v3/orders/${encodeURIComponent(id)}`, ctx)
  };
}
export {
  client_exports as kcov3
};
