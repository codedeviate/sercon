declare const net: any;
import { ClientCtx, buildUrl, rateLimitOf } from "./http";
import { FavroError } from "./errors";

export interface AttachmentInput {
  filename: string;
  content: string | Uint8Array | ArrayBuffer;
  type?: string;
  field?: string; // multipart field name; Favro's exact name is unconfirmed — default "file", overridable.
}

// uploadAttachment POSTs a multipart/form-data body via net.http (which
// assembles the multipart body in Go and sets the Content-Type + boundary).
export async function uploadAttachment(ctx: ClientCtx, path: string, input: AttachmentInput): Promise<any> {
  if (!input || !input.filename) throw new Error("favro uploadAttachment: filename is required");
  if (input.content === undefined || input.content === null) throw new Error("favro uploadAttachment: content is required");
  const headers: Record<string, string> = { accept: "application/json", authorization: ctx.authHeader };
  if (ctx.organizationId) headers["organizationId"] = ctx.organizationId;
  const url = buildUrl(ctx.baseUrl, path);
  const part: any = { name: input.field || "file", filename: input.filename, content: input.content };
  if (input.type) part.type = input.type;
  const res = await net.http.request("POST", url, { headers, multipart: [part], follow: true });
  let parsed: unknown = undefined;
  if (res.body) { try { parsed = JSON.parse(res.body); } catch { parsed = res.body; } }
  if (res.status >= 200 && res.status < 300) return parsed;
  const requestId = typeof (parsed as any)?.requestId === "string" ? (parsed as any).requestId : undefined;
  throw new FavroError(res.status, parsed, requestId, rateLimitOf(res.headers || {}));
}
