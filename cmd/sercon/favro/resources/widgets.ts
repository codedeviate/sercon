import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function widgets(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/widgets" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove };
}
