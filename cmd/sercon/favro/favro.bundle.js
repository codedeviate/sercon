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
function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
function retryWaitMs(h) {
  const ra = h["retry-after"];
  if (ra !== void 0) {
    const s = Number(ra);
    if (!isNaN(s)) return Math.max(0, s * 1e3);
  }
  const reset = h["x-ratelimit-reset"];
  if (reset !== void 0) {
    const t = Date.parse(reset);
    if (!isNaN(t)) return Math.max(0, t - Date.now());
  }
  return 1e3;
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
  const maxAttempts = ctx.retry === false ? 1 : ctx.retry.max + 1;
  let attempt = 0;
  for (; ; ) {
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
    if (res.status === 429 && ctx.retry !== false && attempt < ctx.retry.max) {
      const waitMs = retryWaitMs(respHeaders);
      if (waitMs <= ctx.retry.maxWaitMs) {
        attempt++;
        await sleep(waitMs);
        continue;
      }
    }
    const requestId = typeof parsed?.requestId === "string" ? parsed.requestId : void 0;
    throw new FavroError(res.status, parsed, requestId, rateLimitOf(respHeaders));
  }
}

// cmd/sercon/favro/resources/organizations.ts
function organizations(ctx) {
  return {
    get: async (id) => (await request(ctx, "GET", `/organizations/${encodeURIComponent(id)}`, { orgScoped: false })).body
  };
}

// cmd/sercon/favro/core/pagination.ts
var BACKEND_HEADER = "x-favro-backend-identifier";
async function fetchPage(ctx, path, query, page, requestId, backendId) {
  const q = { ...query };
  if (requestId !== void 0) q.requestId = requestId;
  if (page !== void 0) q.page = page;
  const headers = {};
  if (backendId) headers["X-Favro-Backend-Identifier"] = backendId;
  const res = await request(ctx, "GET", path, { query: q, headers });
  const b = res.body || {};
  return {
    page: {
      entities: b.entities || [],
      page: b.page ?? 0,
      pages: b.pages ?? 1,
      requestId: b.requestId,
      limit: b.limit ?? 100
    },
    backendId: res.headers[BACKEND_HEADER] || backendId
  };
}
async function listAll(ctx, path, query) {
  const first = await fetchPage(ctx, path, query);
  let out = first.page.entities.slice();
  const { pages, requestId } = first.page;
  const backendId = first.backendId;
  for (let p = 1; p < pages; p++) {
    const next = await fetchPage(ctx, path, query, p, requestId, backendId);
    out = out.concat(next.page.entities);
  }
  return out;
}
async function* iterate(ctx, path, query) {
  const first = await fetchPage(ctx, path, query);
  for (const e of first.page.entities) yield e;
  const { pages, requestId } = first.page;
  const backendId = first.backendId;
  for (let p = 1; p < pages; p++) {
    const next = await fetchPage(ctx, path, query, p, requestId, backendId);
    for (const e of next.page.entities) yield e;
  }
}

// cmd/sercon/favro/core/resource.ts
function collection(ctx, d) {
  const orgScoped = d.orgScoped !== false;
  const idPath = (id) => `${d.path}/${encodeURIComponent(id)}`;
  const check = (params) => {
    if (d.validateList) d.validateList(params);
  };
  return {
    async list(params = {}) {
      check(params);
      return (await fetchPage(ctx, d.path, params)).page;
    },
    listAll(params = {}) {
      check(params);
      return listAll(ctx, d.path, params);
    },
    iterate(params = {}) {
      check(params);
      return iterate(ctx, d.path, params);
    },
    async get(id, params = {}) {
      return (await request(ctx, "GET", idPath(id), { query: params, orgScoped })).body;
    },
    async create(body) {
      return (await request(ctx, "POST", d.path, { body, orgScoped })).body;
    },
    async update(id, body) {
      return (await request(ctx, "PUT", idPath(id), { body, orgScoped })).body;
    },
    async remove(id) {
      await request(ctx, "DELETE", idPath(id), { orgScoped });
    }
  };
}

// cmd/sercon/favro/resources/cards.ts
var CARD_SCOPES = ["widgetCommonId", "collectionId", "cardCommonId", "cardSequentialId", "todoList"];
function validateCardList(params) {
  if (!CARD_SCOPES.some((k) => params[k] !== void 0)) {
    throw new Error("favro cards.list: one of " + CARD_SCOPES.join(", ") + " is required");
  }
}
function cards(ctx) {
  const c = collection(ctx, { path: "/cards", validateList: validateCardList });
  return {
    list: c.list,
    listAll: c.listAll,
    iterate: c.iterate,
    get: c.get,
    create: c.create,
    update: c.update,
    remove: c.remove
  };
}

// cmd/sercon/favro/resources/collections.ts
function collections(ctx) {
  const c = collection(ctx, { path: "/collections" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove };
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
    cards: cards(ctx),
    collections: collections(ctx)
  };
}
export {
  client
};
