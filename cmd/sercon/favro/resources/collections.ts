import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function collections(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/collections" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove };
}
