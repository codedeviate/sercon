import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function tags(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/tags" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove };
}
