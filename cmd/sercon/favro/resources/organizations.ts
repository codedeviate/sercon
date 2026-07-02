import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

// organizations is user-level: orgScoped:false means the organizationId
// header is never sent for this group.
export function organizations(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/organizations", orgScoped: false });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update };
}
