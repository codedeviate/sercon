// cmd/sercon/favro/core/config.ts
function envGet(name) {
  const v = runtime.env.get(name);
  return v === void 0 || v === null ? void 0 : String(v);
}
var DEFAULT_BASE_URL = "https://favro.com/api/v1";
function resolveBaseUrl(baseUrl) {
  const b = baseUrl ?? envGet("FAVRO_BASE_URL") ?? DEFAULT_BASE_URL;
  return b.replace(/\/+$/, "");
}

// cmd/sercon/favro/core/errors.ts
var FavroError = class extends Error {
  status;
  body;
  requestId;
  rateLimit;
  constructor(status, body, requestId, rateLimit) {
    super(`favro: HTTP ${status}`);
    this.name = "FavroError";
    this.status = status;
    this.body = body;
    this.requestId = requestId;
    this.rateLimit = rateLimit;
  }
};

// cmd/sercon/favro/core/http.ts
function num(h, k) {
  const v = h[k];
  if (v === void 0) return void 0;
  const n = Number(v);
  return isNaN(n) ? void 0 : n;
}
function rateLimitOf(h) {
  return {
    limit: num(h, "x-ratelimit-limit"),
    remaining: num(h, "x-ratelimit-remaining"),
    reset: h["x-ratelimit-reset"],
    retryAfter: num(h, "retry-after")
  };
}
function buildUrl(base, path, query) {
  let url = path.startsWith("http") ? path : base + path;
  if (query) {
    const parts = [];
    for (const k in query) {
      const v = query[k];
      if (v !== void 0) parts.push(encodeURIComponent(k) + "=" + encodeURIComponent(String(v)));
    }
    if (parts.length) url += (url.includes("?") ? "&" : "?") + parts.join("&");
  }
  return url;
}
async function request(ctx, method, path, opts = {}) {
  const orgScoped = opts.orgScoped !== false;
  const url = buildUrl(ctx.baseUrl, path, opts.query);
  const headers = { accept: "application/json", authorization: ctx.authHeader, ...opts.headers || {} };
  if (orgScoped && ctx.organizationId) headers["organizationId"] = ctx.organizationId;
  let bodyStr;
  if (opts.body !== void 0) {
    headers["content-type"] = "application/json";
    bodyStr = JSON.stringify(opts.body);
  }
  const res = await net.http.request(method, url, { headers, body: bodyStr, follow: true });
  const respHeaders = res.headers || {};
  let parsed = void 0;
  if (res.body) {
    try {
      parsed = JSON.parse(res.body);
    } catch {
      parsed = res.body;
    }
  }
  if (res.status >= 200 && res.status < 300) {
    return { status: res.status, headers: respHeaders, body: parsed };
  }
  const requestId = typeof parsed?.requestId === "string" ? parsed.requestId : void 0;
  throw new FavroError(res.status, parsed, requestId, rateLimitOf(respHeaders));
}

// cmd/sercon/favro/resources/organizations.ts
function organizations(ctx) {
  return {
    get: async (id) => (await request(ctx, "GET", `/organizations/${encodeURIComponent(id)}`, { orgScoped: false })).body
  };
}

// cmd/sercon/favro/resources/cards.ts
function cards(ctx) {
  return {
    get: async (id, params = {}) => (await request(ctx, "GET", `/cards/${encodeURIComponent(id)}`, { query: params })).body
  };
}

// cmd/sercon/favro/index.ts
function resolveRetry(r) {
  if (r === false) return false;
  return { max: r?.max ?? 2, maxWaitMs: r?.maxWaitMs ?? 3e4 };
}
function client(overrides = {}) {
  const email = overrides.email ?? envGet("FAVRO_EMAIL");
  const token = overrides.apiToken ?? overrides.password ?? envGet("FAVRO_API_TOKEN");
  if (!email) throw new Error("favro: FAVRO_EMAIL is required (set it in the environment/.env or pass email)");
  if (!token) throw new Error("favro: FAVRO_API_TOKEN is required (set it in the environment/.env or pass apiToken)");
  const ctx = {
    baseUrl: resolveBaseUrl(overrides.baseUrl),
    authHeader: "Basic " + text.str.base64Encode(`${email}:${token}`),
    organizationId: overrides.organizationId ?? envGet("FAVRO_ORGANIZATION_ID"),
    retry: resolveRetry(overrides.retry)
  };
  return {
    organizations: organizations(ctx),
    cards: cards(ctx)
  };
}
export {
  client
};
