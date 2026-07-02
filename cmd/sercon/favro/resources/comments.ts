import { ClientCtx } from "../core/http";
import { collection } from "../core/resource";
import { uploadAttachment, AttachmentInput } from "../core/attachments";

export function comments(ctx: ClientCtx) {
  const c = collection(ctx, {
    path: "/comments",
    validateList: (p) => { if (p.cardCommonId === undefined) throw new Error("favro comments.list: cardCommonId is required"); },
  });
  return {
    list: c.list, listAll: c.listAll, iterate: c.iterate, get: c.get, create: c.create, update: c.update, remove: c.remove,
    uploadAttachment: (commentId: string, input: AttachmentInput) =>
      uploadAttachment(ctx, `/comments/${encodeURIComponent(commentId)}/attachments`, input),
  };
}
