import { ClientCtx, request } from "../core/http";
import { collection } from "../core/resource";
import { fetchPage } from "../core/pagination";
import { uploadAttachment, AttachmentInput } from "../core/attachments";

const CARD_SCOPES = ["widgetCommonId", "collectionId", "cardCommonId", "cardSequentialId", "todoList"];

function validateCardList(params: Record<string, any>) {
  if (!CARD_SCOPES.some((k) => params[k] !== undefined)) {
    throw new Error("favro cards.list: one of " + CARD_SCOPES.join(", ") + " is required");
  }
}

export function cards(ctx: ClientCtx) {
  const c = collection(ctx, { path: "/cards", validateList: validateCardList });
  const dep = (id: string) => `/cards/${encodeURIComponent(id)}/dependencies`;
  return {
    list: c.list,
    listAll: c.listAll,
    iterate: c.iterate,
    get: c.get,
    create: c.create,
    update: c.update,
    remove: c.remove,
    dependencies: {
      list: async (cardId: string) => (await request(ctx, "GET", dep(cardId))).body,
      add: async (cardId: string, body: Record<string, any>) => (await request(ctx, "POST", dep(cardId), { body })).body,
      set: async (cardId: string, body: Record<string, any>) => (await request(ctx, "PUT", dep(cardId), { body })).body,
      update: async (cardId: string, depId: string, body: Record<string, any>) =>
        (await request(ctx, "PATCH", `${dep(cardId)}/${encodeURIComponent(depId)}`, { body })).body,
      remove: async (cardId: string, depId: string) => { await request(ctx, "DELETE", `${dep(cardId)}/${encodeURIComponent(depId)}`); },
      removeAll: async (cardId: string) => { await request(ctx, "DELETE", dep(cardId)); },
    },
    activities: {
      list: async (cardId: string, params: Record<string, any> = {}) =>
        (await fetchPage(ctx, `/cards/${encodeURIComponent(cardId)}/activities`, params)).page,
    },
    uploadAttachment: (cardId: string, input: AttachmentInput) =>
      uploadAttachment(ctx, `/cards/${encodeURIComponent(cardId)}/attachments`, input),
  };
}
