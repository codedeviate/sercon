import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

function requireParam(name: string) {
  return (params: Record<string, any>) => {
    if (params[name] === undefined) throw new Error(`favro columns.list: ${name} is required`);
  };
}

export function columns(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/columns", validateList: requireParam("widgetCommonId") });
  return { list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove };
}
