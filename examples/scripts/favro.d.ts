// Type declarations for the sercon-bundled `favro` library.
// Hand-maintained (the library is an embedded module, not a reserved global).
//
// Conventions:
//  - `client(overrides?)` is synchronous and throws a plain Error if email or
//    apiToken is missing. Config precedence is `overrides.X ?? env(FAVRO_X)`.
//  - organizationId is fixed on the client and auto-sent on org-scoped routes.
//  - List methods paginate: `list` returns one page envelope, `listAll`
//    returns all entities, `iterate` streams them (`for await`).
//  - Methods return parsed JSON as-is. Non-2xx throws FavroError
//    { status, body, requestId?, rateLimit? }. 429 is retried (bounded, opt-out).
declare module "favro" {
  interface Page<T = any> { entities: T[]; page: number; pages: number; requestId: string; limit: number; }
  interface RateLimitInfo { limit?: number; remaining?: number; reset?: string; retryAfter?: number; }
  interface AttachmentInput { filename: string; content: string | Uint8Array | ArrayBuffer; type?: string; field?: string; }

  interface Collection<T = any> {
    list(params?: Record<string, any>): Promise<Page<T>>;
    listAll(params?: Record<string, any>): Promise<T[]>;
    iterate(params?: Record<string, any>): AsyncIterableIterator<T>;
    get(id: string, params?: Record<string, any>): Promise<T>;
    create(body: Record<string, any>): Promise<T>;
    update(id: string, body: Record<string, any>): Promise<T>;
    remove(id: string): Promise<void>;
  }

  interface CardsGroup extends Collection {
    dependencies: {
      list(cardId: string): Promise<any>;
      add(cardId: string, body: Record<string, any>): Promise<any>;
      set(cardId: string, body: Record<string, any>): Promise<any>;
      update(cardId: string, depId: string, body: Record<string, any>): Promise<any>;
      remove(cardId: string, depId: string): Promise<void>;
      removeAll(cardId: string): Promise<void>;
    };
    activities: { list(cardId: string, params?: Record<string, any>): Promise<Page> };
    uploadAttachment(cardId: string, input: AttachmentInput): Promise<any>;
  }

  interface CommentsGroup extends Collection {
    uploadAttachment(commentId: string, input: AttachmentInput): Promise<any>;
  }

  interface FavroClient {
    organizations: Pick<Collection, "list" | "listAll" | "iterate" | "get" | "create" | "update">;
    users: Pick<Collection, "list" | "listAll" | "iterate" | "get">;
    collections: Collection;
    widgets: Collection;
    columns: Collection;
    cards: CardsGroup;
    comments: CommentsGroup;
    tasks: Collection;
    tasklists: Collection;
    tags: Collection;
    customFields: Pick<Collection, "list" | "listAll" | "iterate" | "get">;
    groups: Collection;
    webhooks: Pick<Collection, "list" | "listAll" | "iterate" | "create" | "remove">;
  }

  interface FavroConfig {
    email?: string;
    apiToken?: string;
    password?: string;
    organizationId?: string;
    baseUrl?: string;
    retry?: { max?: number; maxWaitMs?: number } | false;
  }

  export function client(overrides?: FavroConfig): FavroClient;
}
