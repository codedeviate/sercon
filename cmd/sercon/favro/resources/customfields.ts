import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

export function customFields(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/customfields" });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get };
}
