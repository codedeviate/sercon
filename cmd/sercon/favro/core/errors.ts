export interface RateLimitInfo {
  limit?: number;
  remaining?: number;
  reset?: string;
  retryAfter?: number;
}

export class FavroError extends Error {
  status: number;
  body: unknown;
  requestId?: string;
  rateLimit?: RateLimitInfo;
  constructor(status: number, body: unknown, requestId?: string, rateLimit?: RateLimitInfo) {
    super(`favro: HTTP ${status}`);
    this.name = "FavroError";
    this.status = status;
    this.body = body;
    this.requestId = requestId;
    this.rateLimit = rateLimit;
  }
}
