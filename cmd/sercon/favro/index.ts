declare const text: any;
import { envGet, resolveBaseUrl } from "./core/config";
import { ClientCtx, RetryConfig } from "./core/http";
import { organizations } from "./resources/organizations";
import { cards } from "./resources/cards";
import { collections } from "./resources/collections";
import { widgets } from "./resources/widgets";
import { columns } from "./resources/columns";
import { tasks } from "./resources/tasks";
import { tasklists } from "./resources/tasklists";
import { tags } from "./resources/tags";
import { groups } from "./resources/groups";
import { users } from "./resources/users";

export interface FavroConfig {
  email?: string;
  apiToken?: string;
  password?: string;
  organizationId?: string;
  baseUrl?: string;
  retry?: { max?: number; maxWaitMs?: number } | false;
}

function resolveRetry(r: FavroConfig["retry"]): RetryConfig | false {
  if (r === false) return false;
  return { max: r?.max ?? 2, maxWaitMs: r?.maxWaitMs ?? 30000 };
}

export function client(overrides: FavroConfig = {}) {
  const email = overrides.email ?? envGet("FAVRO_EMAIL");
  const token = overrides.apiToken ?? overrides.password ?? envGet("FAVRO_API_TOKEN");
  if (!email) throw new Error("favro: FAVRO_EMAIL is required (set it in the environment/.env or pass email)");
  if (!token) throw new Error("favro: FAVRO_API_TOKEN is required (set it in the environment/.env or pass apiToken)");
  const ctx: ClientCtx = {
    baseUrl: resolveBaseUrl(overrides.baseUrl),
    authHeader: "Basic " + text.str.base64Encode(`${email}:${token}`),
    organizationId: overrides.organizationId ?? envGet("FAVRO_ORGANIZATION_ID"),
    retry: resolveRetry(overrides.retry),
  };
  return {
    organizations: organizations(ctx),
    cards: cards(ctx),
    collections: collections(ctx),
    widgets: widgets(ctx),
    columns: columns(ctx),
    tasks: tasks(ctx),
    tasklists: tasklists(ctx),
    tags: tags(ctx),
    groups: groups(ctx),
    users: users(ctx),
  };
}
