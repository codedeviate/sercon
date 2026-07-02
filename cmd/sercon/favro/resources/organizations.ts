import { ClientCtx, request } from "../core/http";

// organizations is user-level: the organizationId header is NOT sent.
export function organizations(ctx: ClientCtx) {
  return {
    get: async (id: string) => (await request(ctx, "GET", `/organizations/${encodeURIComponent(id)}`, { orgScoped: false })).body,
  };
}
