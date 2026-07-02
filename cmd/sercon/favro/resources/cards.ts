import { ClientCtx, request } from "../core/http";

export function cards(ctx: ClientCtx) {
  return {
    get: async (id: string, params: Record<string, any> = {}) =>
      (await request(ctx, "GET", `/cards/${encodeURIComponent(id)}`, { query: params })).body,
  };
}
