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
  const url = path.indexOf("http") === 0 ? path : ctx.baseUrl + path;
  const res = await net.http.request(method, url, {
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
var B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
function base64Bytes(bytes) {
  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const b0 = bytes[i];
    const b1 = i + 1 < bytes.length ? bytes[i + 1] : 0;
    const b2 = i + 2 < bytes.length ? bytes[i + 2] : 0;
    out += B64[b0 >> 2];
    out += B64[(b0 & 3) << 4 | b1 >> 4];
    out += i + 1 < bytes.length ? B64[(b1 & 15) << 2 | b2 >> 6] : "=";
    out += i + 2 < bytes.length ? B64[b2 & 63] : "=";
  }
  return out;
}
function hexToBytes(hex) {
  const out = [];
  for (let i = 0; i + 1 < hex.length; i += 2) out.push(parseInt(hex.substr(i, 2), 16));
  return out;
}
function sha256Base64(input) {
  return base64Bytes(hexToBytes(crypto.hash.sha256(input)));
}
function sha512HexUpper(input) {
  return crypto.hash.sha512(input).toUpperCase();
}
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

// cmd/sercon/paymentproviders/netsv1/client.ts
var client_exports2 = {};
__export(client_exports2, {
  client: () => client2
});
var TEST_URL2 = "https://test.api.dibspayment.eu";
var PROD_URL2 = "https://api.dibspayment.eu";
function client2(overrides = {}) {
  const secretKey = overrides.secretKey ?? envGet("NETS_SECRET_KEY");
  if (!secretKey) throw new Error("netsv1: NETS_SECRET_KEY is required (set it in the environment/.env or pass secretKey)");
  const env = overrides.env ?? envGet("NETS_ENV");
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("NETS_BASE_URL"), TEST_URL2, PROD_URL2);
  const ctx = { baseUrl, provider: "netsv1", sign: () => ({ Authorization: secretKey }) };
  const p = (id) => `/v1/payments/${encodeURIComponent(id)}`;
  return {
    createPayment: (payment) => apiRequest("POST", "/v1/payments", ctx, payment),
    getPayment: (id) => apiRequest("GET", p(id), ctx),
    // SEAM: charge/refund body shape ({ amount, orderItems }) — confirm against docs/live.
    capturePayment: (id, input) => apiRequest("POST", `${p(id)}/charges`, ctx, { amount: input.amount, orderItems: input.orderItems }),
    refundPayment: (id, input) => apiRequest("POST", `${p(id)}/refunds`, ctx, { amount: input.amount, orderItems: input.orderItems }),
    cancelPayment: (id) => apiRequest("PUT", `${p(id)}/terminate`, ctx)
  };
}

// cmd/sercon/paymentproviders/sveacheckout2/client.ts
var client_exports3 = {};
__export(client_exports3, {
  client: () => client3
});
var TEST_URL3 = "https://checkoutapistage.svea.com";
var PROD_URL3 = "https://checkoutapi.svea.com";
function sveaTimestamp() {
  return (/* @__PURE__ */ new Date()).toISOString().replace("T", " ").split(".")[0];
}
function client3(overrides = {}) {
  const merchantId = overrides.merchantId ?? envGet("SCO_MERCHANT_ID");
  const secretKey = overrides.secretKey ?? envGet("SCO_SECRET_KEY");
  if (!merchantId) throw new Error("sveacheckout2: SCO_MERCHANT_ID is required (set it in the environment/.env or pass merchantId)");
  if (!secretKey) throw new Error("sveacheckout2: SCO_SECRET_KEY is required (set it in the environment/.env or pass secretKey)");
  const env = overrides.env ?? envGet("SCO_ENV");
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("SCO_BASE_URL"), TEST_URL3, PROD_URL3);
  const sign = (_m, _p, bodyStr) => {
    const ts = sveaTimestamp();
    const hash = sha512HexUpper(bodyStr + secretKey + ts);
    const token = text.str.base64Encode(`${merchantId}:${hash}`);
    return { Authorization: "Svea " + token, Timestamp: ts };
  };
  const ctx = { baseUrl, provider: "sveacheckout2", sign };
  const ord = (id) => `/api/orders/${encodeURIComponent(id)}`;
  return {
    createOrder: (order) => apiRequest("POST", "/api/orders", ctx, order),
    getOrder: (id) => apiRequest("GET", ord(id), ctx),
    getPayment: (id) => apiRequest("GET", ord(id), ctx),
    // SEAM paths below — confirmed/fixed live later.
    capturePayment: (id, input) => apiRequest("POST", `${ord(id)}/deliveries`, ctx, { amount: input.amount, rows: input.rows }),
    refundPayment: (id, input) => apiRequest("POST", `${ord(id)}/credits`, ctx, { amount: input.amount, rows: input.rows }),
    cancelPayment: (id) => apiRequest("POST", `${ord(id)}/cancel`, ctx)
  };
}

// cmd/sercon/paymentproviders/qlirov2/client.ts
var client_exports4 = {};
__export(client_exports4, {
  client: () => client4
});
var TEST_URL4 = "https://pago.qit.nu";
var PROD_URL4 = "https://payments.qit.nu";
function uuidV4() {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = Math.random() * 16 | 0;
    const v = c === "x" ? r : r & 3 | 8;
    return v.toString(16);
  });
}
function client4(overrides = {}) {
  const apiKey = overrides.apiKey ?? envGet("QLIRO_API_KEY");
  const apiPassword = overrides.apiPassword ?? envGet("QLIRO_APIPASSWORD");
  if (!apiKey) throw new Error("qlirov2: QLIRO_API_KEY is required (set it in the environment/.env or pass apiKey)");
  if (!apiPassword) throw new Error("qlirov2: QLIRO_APIPASSWORD is required (set it in the environment/.env or pass apiPassword)");
  const env = overrides.env ?? envGet("QLIRO_ENV");
  const baseUrl = pickBaseUrl(env, overrides.baseUrl ?? envGet("QLIRO_BASE_URL"), TEST_URL4, PROD_URL4);
  const sign = (_m, _p, bodyStr) => ({ Authorization: "Qliro " + sha256Base64(bodyStr + apiPassword) });
  const ctx = { baseUrl, provider: "qlirov2", sign };
  const mgmt = (orderId, extra) => ({ RequestId: uuidV4(), OrderId: orderId, ...extra, MerchantApiKey: apiKey });
  const oid = (id) => `/checkout/merchantapi/Orders/${encodeURIComponent(String(id))}`;
  return {
    // createOrder: MerchantApiKey injected last so a caller's order cannot override it.
    createOrder: (order) => apiRequest("POST", "/checkout/merchantapi/Orders", ctx, { ...order, MerchantApiKey: apiKey }),
    // getOrder is RESTful: GET /Orders/{id}, no body. getPayment is an alias (the order IS the payment).
    getOrder: (id) => apiRequest("GET", oid(id), ctx),
    getPayment: (id) => apiRequest("GET", oid(id), ctx),
    // Admin API v2 management: capture = MarkItemsAsShipped, refund = ReturnItems, cancel = cancelOrder.
    capturePayment: (orderId, input = {}) => apiRequest("POST", "/checkout/adminapi/v2/MarkItemsAsShipped", ctx, mgmt(orderId, input)),
    refundPayment: (orderId, input = {}) => apiRequest("POST", "/checkout/adminapi/v2/ReturnItems", ctx, mgmt(orderId, input)),
    cancelPayment: (orderId) => apiRequest("POST", "/checkout/adminapi/v2/cancelOrder", ctx, mgmt(orderId, {}))
  };
}
export {
  client_exports as kcov3,
  client_exports2 as netsv1,
  client_exports4 as qlirov2,
  client_exports3 as sveacheckout2
};
