import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function users(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/users" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get };
}
