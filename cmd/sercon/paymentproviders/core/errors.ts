export class PaymentError extends Error {
  provider: string;
  status: number;
  body: unknown;
  requestId?: string;
  constructor(provider: string, status: number, body: unknown, requestId?: string) {
    super(`${provider}: HTTP ${status}`);
    this.name = "PaymentError";
    this.provider = provider;
    this.status = status;
    this.body = body;
    this.requestId = requestId;
  }
}
