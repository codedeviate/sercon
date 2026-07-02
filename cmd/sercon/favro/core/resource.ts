import { ClientCtx, request } from "./http";
import { Page, fetchPage, listAll, iterate } from "./pagination";

export interface ResourceDescriptor {
  path: string;                                  // e.g. "/cards"
  validateList?: (params: Record<string, any>) => void; // throws for invalid scoped-list params
  orgScoped?: boolean;                           // default true
}

export interface Collection<T> {
  list(params?: Record<string, any>): Promise<Page<T>>;
  listAll(params?: Record<string, any>): Promise<T[]>;
  iterate(params?: Record<string, any>): AsyncIterableIterator<T>;
  get(id: string, params?: Record<string, any>): Promise<T>;
  create(body: Record<string, any>): Promise<T>;
  update(id: string, body: Record<string, any>): Promise<T>;
  remove(id: string): Promise<void>;
}

// collection builds the standard CRUD + pagination surface from a descriptor.
// Resource files expose the subset that the API actually supports by
// destructuring the returned object (methods are closures, safe to detach).
export function collection<T = any>(ctx: ClientCtx, d: ResourceDescriptor): Collection<T> {
  const orgScoped = d.orgScoped !== false;
  const idPath = (id: string) => `${d.path}/${encodeURIComponent(id)}`;
  const check = (params: Record<string, any>) => { if (d.validateList) d.validateList(params); };
  return {
    async list(params = {}) { check(params); return (await fetchPage<T>(ctx, d.path, params, undefined, undefined, undefined, orgScoped)).page; },
    listAll(params = {}) { check(params); return listAll<T>(ctx, d.path, params, orgScoped); },
    iterate(params = {}) { check(params); return iterate<T>(ctx, d.path, params, orgScoped); },
    async get(id, params = {}) { return (await request(ctx, "GET", idPath(id), { query: params, orgScoped })).body as T; },
    async create(body) { return (await request(ctx, "POST", d.path, { body, orgScoped })).body as T; },
    async update(id, body) { return (await request(ctx, "PUT", idPath(id), { body, orgScoped })).body as T; },
    async remove(id) { await request(ctx, "DELETE", idPath(id), { orgScoped }); },
  };
}
