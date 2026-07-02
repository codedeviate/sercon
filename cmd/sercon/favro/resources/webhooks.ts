import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function webhooks(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/webhooks" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, create: c.create, remove: c.remove };
}
