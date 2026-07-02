import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function tasklists(ctx: ClientCtx) {
  const c = collection(ctx, {
    path: "/tasklists",
    validateList: (p) => { if (p.cardCommonId === undefined) throw new Error("favro tasklists.list: cardCommonId is required"); },
  });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove };
}
