import { ClientCtx, request } from "./http";

export interface Page<T> {
  entities: T[];
  page: number;
  pages: number;
  requestId: string;
  limit: number;
}

const BACKEND_HEADER = "x-favro-backend-identifier";

// fetchPage fetches one page. For page > 0, pass the requestId from page 0 and
// the backendId captured from page 0's response headers (pins the request to
// the same Favro backend process). Returns the page envelope plus the backend
// id to reuse for subsequent pages.
export async function fetchPage<T>(
  ctx: ClientCtx,
  path: string,
  query: Record<string, any>,
  page?: number,
  requestId?: string,
  backendId?: string,
): Promise<{ page: Page<T>; backendId?: string }> {
  const q = { ...query };
  if (requestId !== undefined) q.requestId = requestId;
  if (page !== undefined) q.page = page;
  const headers: Record<string, string> = {};
  if (backendId) headers["X-Favro-Backend-Identifier"] = backendId;
  const res = await request(ctx, "GET", path, { query: q, headers });
  const b = (res.body || {}) as any;
  return {
    page: {
      entities: b.entities || [],
      page: b.page ?? 0,
      pages: b.pages ?? 1,
      requestId: b.requestId,
      limit: b.limit ?? 100,
    },
    backendId: res.headers[BACKEND_HEADER] || backendId,
  };
}

export async function listAll<T>(ctx: ClientCtx, path: string, query: Record<string, any>): Promise<T[]> {
  const first = await fetchPage<T>(ctx, path, query);
  let out = first.page.entities.slice();
  const { pages, requestId } = first.page;
  const backendId = first.backendId;
  for (let p = 1; p < pages; p++) {
    const next = await fetchPage<T>(ctx, path, query, p, requestId, backendId);
    out = out.concat(next.page.entities);
  }
  return out;
}

export async function* iterate<T>(ctx: ClientCtx, path: string, query: Record<string, any>): AsyncIterableIterator<T> {
  const first = await fetchPage<T>(ctx, path, query);
  for (const e of first.page.entities) yield e;
  const { pages, requestId } = first.page;
  const backendId = first.backendId;
  for (let p = 1; p < pages; p++) {
    const next = await fetchPage<T>(ctx, path, query, p, requestId, backendId);
    for (const e of next.page.entities) yield e;
  }
}
