import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";

const CARD_SCOPES = ["widgetCommonId", "collectionId", "cardCommonId", "cardSequentialId", "todoList"];

function validateCardList(params: Record<string, any>) {
  if (!CARD_SCOPES.some((k) => params[k] !== undefined)) {
    throw new Error("favro cards.list: one of " + CARD_SCOPES.join(", ") + " is required");
  }
}

export function cards(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/cards", validateList: validateCardList });
  return {
    list: c.list,
    listAll: c.listAll,
    iterate: c.iterate,
    get: c.get,
    create: c.create,
    update: c.update,
    remove: c.remove,
  };
}
