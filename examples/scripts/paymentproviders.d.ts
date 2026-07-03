// Type declarations for the sercon-bundled `paymentproviders` library.
// Hand-maintained (the library is an embedded module, not a reserved global).
//
// Shared conventions across every provider namespace:
//  - `client(overrides?)` is synchronous and throws a plain Error if a required
//    credential is missing. Config precedence is `overrides.X ?? env(PROVIDER_X)`.
//  - The base URL defaults to the provider's TEST endpoint; `env: "prod"` (or
//    `PROVIDER_ENV=prod`) selects production. An explicit `baseUrl` always wins.
//  - Amounts are integer minor units (öre / cents).
//  - Client methods are async and return the parsed JSON response. A non-2xx
//    response throws a `PaymentError` { provider, status, body, requestId? }.
declare module "paymentproviders" {
  export namespace kcov3 {
    interface KcoConfig {
      merchantId?: string;
      sharedSecret?: string;
      env?: "test" | "prod";
      baseUrl?: string;
    }
    interface CaptureInput { amount: number; orderLines?: unknown[]; description?: string; }
    interface RefundInput { amount: number; orderLines?: unknown[]; description?: string; }
    interface KcoIdempotencyOpts {
      /**
       * Pin the `klarna-idempotency-key` header to this exact value instead of
       * auto-generating one. Reuse the same value across retries of the same
       * logical mutation so Klarna dedupes instead of double-processing it.
       */
      idempotencyKey?: string;
    }
    interface KcoClient {
      /**
       * Fetch an order-management order.
       * @param id Kustom order id.
       * @returns The parsed order-management order.
       * @throws PaymentError on a non-2xx response.
       */
      getPayment(id: string): Promise<any>;
      /**
       * Acknowledge an order (POST, idempotency-keyed).
       * @param id Kustom order id.
       * @param opts Optional `{ idempotencyKey }` to pin the retry key instead
       *   of auto-generating one.
       * @returns The acknowledge result.
       * @throws PaymentError on a non-2xx response.
       */
      acknowledge(id: string, opts?: KcoIdempotencyOpts): Promise<any>;
      /**
       * Capture (fully or partially) an authorized order.
       * @param id Kustom order id.
       * @param input Capture input; `amount` is sent as `captured_amount`,
       *   `orderLines` as `order_lines`. Minor units (öre).
       * @param opts Optional `{ idempotencyKey }` to pin the retry key instead
       *   of auto-generating one.
       * @returns The capture result.
       * @throws PaymentError on a non-2xx response.
       */
      capturePayment(id: string, input: CaptureInput, opts?: KcoIdempotencyOpts): Promise<any>;
      /**
       * Refund a captured order.
       * @param id Kustom order id.
       * @param input Refund input; `amount` is sent as `refunded_amount`,
       *   `orderLines` as `order_lines`. Minor units (öre).
       * @param opts Optional `{ idempotencyKey }` to pin the retry key instead
       *   of auto-generating one.
       * @returns The refund result.
       * @throws PaymentError on a non-2xx response.
       */
      refundPayment(id: string, input: RefundInput, opts?: KcoIdempotencyOpts): Promise<any>;
      /**
       * Cancel an order (releases the full authorization).
       * @param id Kustom order id.
       * @param opts Optional `{ idempotencyKey }` to pin the retry key instead
       *   of auto-generating one.
       * @returns The cancel result.
       * @throws PaymentError on a non-2xx response.
       */
      cancelPayment(id: string, opts?: KcoIdempotencyOpts): Promise<any>;
      /**
       * Release the remaining (uncaptured) authorization on an order.
       * @param id Kustom order id.
       * @param opts Optional `{ idempotencyKey }` to pin the retry key instead
       *   of auto-generating one.
       * @returns The release result.
       * @throws PaymentError on a non-2xx response.
       */
      releaseRemainingAuthorization(id: string, opts?: KcoIdempotencyOpts): Promise<any>;
      /**
       * Create a Checkout v3 order.
       * @param order The checkout order body.
       * @returns The created checkout order.
       * @throws PaymentError on a non-2xx response.
       */
      createCheckout(order: Record<string, unknown>): Promise<any>;
      /**
       * Fetch a Checkout v3 order.
       * @param id Checkout order id.
       * @returns The checkout order.
       * @throws PaymentError on a non-2xx response.
       */
      getCheckout(id: string): Promise<any>;
    }
    /**
     * Build a KCO v3 (Kustom) client. Auth is HTTP Basic
     * (`merchantId:sharedSecret`); POST mutations carry a
     * `klarna-idempotency-key` that is auto-generated unless the call's
     * `opts.idempotencyKey` pins it. Base URLs: test
     * `api.playground.kustom.co`, prod `api.kustom.co`.
     * @param overrides Optional config; each field falls back to the
     *   environment: `KCO_MERCHANT_ID`, `KCO_SHARED_SECRET`, `KCO_ENV`,
     *   `KCO_BASE_URL`.
     * @returns A KCO client.
     * @throws Error if `merchantId` / `sharedSecret` are neither passed nor in env.
     */
    function client(overrides?: KcoConfig): KcoClient;
  }
  export namespace netsv1 {
    interface NetsConfig { secretKey?: string; env?: "test" | "prod"; baseUrl?: string; }
    interface NetsAmountInput { amount: number; orderItems?: unknown[]; }
    interface NetsClient {
      /**
       * Create a payment.
       * @param payment The full payment-create body.
       * @returns The created payment (including `paymentId`).
       * @throws PaymentError on a non-2xx response.
       */
      createPayment(payment: Record<string, unknown>): Promise<any>;
      /**
       * Fetch a payment.
       * @param id Nets payment id.
       * @returns The payment.
       * @throws PaymentError on a non-2xx response.
       */
      getPayment(id: string): Promise<any>;
      /**
       * Charge (capture) a payment.
       * @param id Nets payment id.
       * @param input `{ amount, orderItems? }`, posted to `/charges`. Minor units.
       * @returns The charge result.
       * @throws PaymentError on a non-2xx response.
       */
      capturePayment(id: string, input: NetsAmountInput): Promise<any>;
      /**
       * Refund a charged payment.
       * @param id Nets payment id.
       * @param input `{ amount, orderItems? }`, posted to `/refunds`. Minor units.
       * @returns The refund result.
       * @throws PaymentError on a non-2xx response.
       */
      refundPayment(id: string, input: NetsAmountInput): Promise<any>;
      /**
       * Terminate (cancel) a payment (PUT to `/terminate`).
       * @param id Nets payment id.
       * @returns The termination result.
       * @throws PaymentError on a non-2xx response.
       */
      cancelPayment(id: string): Promise<any>;
    }
    /**
     * Build a Nexi/Nets Checkout Payment API v1 client. Auth: the secret key is
     * sent verbatim as the `Authorization` header (no `Bearer` prefix). Base
     * URLs: test `test.api.dibspayment.eu`, prod `api.dibspayment.eu`.
     * @param overrides Optional config; falls back to `NETS_SECRET_KEY`,
     *   `NETS_ENV`, `NETS_BASE_URL`.
     * @returns A Nets client.
     * @throws Error if `secretKey` is neither passed nor in env.
     */
    function client(overrides?: NetsConfig): NetsClient;
  }
  export namespace sveacheckout2 {
    interface SveaConfig { merchantId?: string; secretKey?: string; env?: "test" | "prod"; baseUrl?: string; }
    interface SveaClient {
      /**
       * Create an order.
       * @param order The order body (POST `/api/orders`).
       * @returns The created order.
       * @throws PaymentError on a non-2xx response.
       */
      createOrder(order: Record<string, unknown>): Promise<any>;
      /**
       * Fetch an order.
       * @param id Svea order id.
       * @returns The order.
       * @throws PaymentError on a non-2xx response.
       */
      getOrder(id: string): Promise<any>;
      /**
       * Fetch the order/payment (alias of `getOrder` — the order is the payment).
       * @param id Svea order id.
       * @returns The order.
       * @throws PaymentError on a non-2xx response.
       */
      getPayment(id: string): Promise<any>;
      /**
       * Deliver (capture) an order.
       * @param id Svea order id.
       * @param input `{ amount, rows? }`, posted to `/deliveries`. Minor units.
       * @returns The delivery result.
       * @throws PaymentError on a non-2xx response.
       */
      capturePayment(id: string, input: { amount: number; rows?: unknown[] }): Promise<any>;
      /**
       * Credit (refund) an order.
       * @param id Svea order id.
       * @param input `{ amount, rows? }`, posted to `/credits`. Minor units.
       * @returns The credit result.
       * @throws PaymentError on a non-2xx response.
       */
      refundPayment(id: string, input: { amount: number; rows?: unknown[] }): Promise<any>;
      /**
       * Cancel an order (POST `/cancel`).
       * @param id Svea order id.
       * @returns The cancel result.
       * @throws PaymentError on a non-2xx response.
       */
      cancelPayment(id: string): Promise<any>;
    }
    /**
     * Build a Svea Checkout client. Auth is body-signed:
     * `Authorization: Svea base64(merchantId:UPPER(SHA512(body+secret+timestamp)))`
     * plus a `Timestamp` header. Base URLs: test `checkoutapistage.svea.com`,
     * prod `checkoutapi.svea.com`.
     * @param overrides Optional config; falls back to `SCO_MERCHANT_ID`,
     *   `SCO_SECRET_KEY`, `SCO_ENV`, `SCO_BASE_URL`.
     * @returns A Svea client.
     * @throws Error if `merchantId` / `secretKey` are neither passed nor in env.
     */
    function client(overrides?: SveaConfig): SveaClient;
  }
  export namespace qlirov2 {
    interface QliroConfig { apiKey?: string; apiPassword?: string; env?: "test" | "prod"; baseUrl?: string; }
    interface QliroClient {
      /**
       * Create an order. `MerchantApiKey` is injected last so a caller's order
       * cannot override it.
       * @param order The order body.
       * @returns The created order.
       * @throws PaymentError on a non-2xx response.
       */
      createOrder(order: Record<string, unknown>): Promise<any>;
      /**
       * Fetch an order (RESTful GET, no body).
       * @param id Qliro order id.
       * @returns The order.
       * @throws PaymentError on a non-2xx response.
       */
      getOrder(id: string | number): Promise<any>;
      /**
       * Fetch the order/payment (alias of `getOrder`).
       * @param id Qliro order id.
       * @returns The order.
       * @throws PaymentError on a non-2xx response.
       */
      getPayment(id: string | number): Promise<any>;
      /**
       * Capture via the Admin API v2 `MarkItemsAsShipped`. The library injects
       * `RequestId`, `OrderId`, and (last) `MerchantApiKey` into the signed body.
       * @param orderId Qliro order id.
       * @param input Optional extra body fields (e.g. item actions).
       * @returns The capture result.
       * @throws PaymentError on a non-2xx response.
       */
      capturePayment(orderId: string | number, input?: Record<string, unknown>): Promise<any>;
      /**
       * Refund via the Admin API v2 `ReturnItems`. Body augmented as for capture.
       * @param orderId Qliro order id.
       * @param input Optional extra body fields.
       * @returns The refund result.
       * @throws PaymentError on a non-2xx response.
       */
      refundPayment(orderId: string | number, input?: Record<string, unknown>): Promise<any>;
      /**
       * Cancel via the Admin API v2 `cancelOrder`.
       * @param orderId Qliro order id.
       * @returns The cancel result.
       * @throws PaymentError on a non-2xx response.
       */
      cancelPayment(orderId: string | number): Promise<any>;
    }
    /**
     * Build a Qliro One client. Auth is body-signed:
     * `Authorization: Qliro base64(SHA256(body+apiPassword))` (for GETs the body
     * string is empty, so the token signs just the secret). Base URLs: test
     * `pago.qit.nu`, prod `payments.qit.nu`.
     * @param overrides Optional config; falls back to `QLIRO_API_KEY`,
     *   `QLIRO_APIPASSWORD`, `QLIRO_ENV`, `QLIRO_BASE_URL`.
     * @returns A Qliro client.
     * @throws Error if `apiKey` / `apiPassword` are neither passed nor in env.
     */
    function client(overrides?: QliroConfig): QliroClient;
  }
  export namespace swedbankpayv3 {
    interface SwedbankPayConfig { accessToken?: string; merchantId?: string; env?: "test" | "prod"; baseUrl?: string; }
    interface SwedbankPayClient {
      /** The resolved merchant id (the `payee.payeeId`). */
      merchantId: string;
      /**
       * Create a payment order (POST `/psp/paymentorders`).
       * @param body The full v3 payment-order body.
       * @returns The created payment order (with a HAL `operations` array).
       * @throws PaymentError on a non-2xx response.
       */
      createPaymentOrder(body: Record<string, unknown>): Promise<any>;
      /**
       * Fetch a payment order.
       * @param idOrUrl A payment-order id or an absolute URL.
       * @returns The payment order.
       * @throws PaymentError on a non-2xx response.
       */
      getPaymentOrder(idOrUrl: string): Promise<any>;
      /**
       * Fetch the payment order (alias of `getPaymentOrder`).
       * @param idOrUrl A payment-order id or an absolute URL.
       * @returns The payment order.
       * @throws PaymentError on a non-2xx response.
       */
      getPayment(idOrUrl: string): Promise<any>;
      /**
       * Resolve a HAL operation by rel and call its `href`. Accepts an
       * already-fetched payment-order payload, or an id/URL to GET first. Rel
       * matching is exact or by hyphen-suffix (so `"capture"` matches
       * `"create-paymentorder-capture"`).
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param rel The HAL operation rel to resolve.
       * @param body Optional request body for the operation.
       * @returns The operation's response.
       * @throws Error if the rel is not available on the payment order;
       *   PaymentError on a non-2xx response.
       */
      operation(paymentOrderOrUrl: string | Record<string, unknown>, rel: string, body?: unknown): Promise<any>;
      /**
       * Capture via the `capture` HAL operation.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param body The capture body (`{ transaction: {...} }`).
       * @returns The capture result.
       * @throws Error if the `capture` rel is absent; PaymentError on non-2xx.
       */
      capturePayment(paymentOrderOrUrl: string | Record<string, unknown>, body: Record<string, unknown>): Promise<any>;
      /**
       * Refund via the `reversal` HAL operation.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param body The reversal body.
       * @returns The reversal result.
       * @throws Error if the `reversal` rel is absent; PaymentError on non-2xx.
       */
      refundPayment(paymentOrderOrUrl: string | Record<string, unknown>, body: Record<string, unknown>): Promise<any>;
      /**
       * Cancel via the `cancel` HAL operation.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param body Optional cancel body.
       * @returns The cancel result.
       * @throws Error if the `cancel` rel is absent; PaymentError on non-2xx.
       */
      cancelPayment(paymentOrderOrUrl: string | Record<string, unknown>, body?: Record<string, unknown>): Promise<any>;
    }
    /**
     * Build a SwedbankPay Checkout v3 client. Auth: `Authorization: Bearer
     * <accessToken>`; responses are HAL/hypermedia (an `operations` array drives
     * capture/reversal/cancel). Base URLs: test
     * `api.externalintegration.payex.com`, prod `api.payex.com`.
     * @param overrides Optional config; falls back to
     *   `SWEDBANKPAY_ACCESS_TOKEN`, `SWEDBANKPAY_MERCHANT_ID`,
     *   `SWEDBANKPAY_ENV`, `SWEDBANKPAY_BASE_URL`.
     * @returns A SwedbankPay v3 client.
     * @throws Error if `accessToken` / `merchantId` are neither passed nor in env.
     */
    function client(overrides?: SwedbankPayConfig): SwedbankPayClient;
  }
  export namespace swedbankpayv2 {
    interface SwedbankPayConfig { accessToken?: string; merchantId?: string; env?: "test" | "prod"; baseUrl?: string; }
    interface SwedbankPayClient {
      /** The resolved merchant id (the `payee.payeeId`). */
      merchantId: string;
      /**
       * Create a payment order (POST `/psp/paymentorders`).
       * @param body The full v2 payment-order body.
       * @returns The created payment order (with a HAL `operations` array).
       * @throws PaymentError on a non-2xx response.
       */
      createPaymentOrder(body: Record<string, unknown>): Promise<any>;
      /**
       * Fetch a payment order.
       * @param idOrUrl A payment-order id or an absolute URL.
       * @returns The payment order.
       * @throws PaymentError on a non-2xx response.
       */
      getPaymentOrder(idOrUrl: string): Promise<any>;
      /**
       * Fetch the payment order (alias of `getPaymentOrder`).
       * @param idOrUrl A payment-order id or an absolute URL.
       * @returns The payment order.
       * @throws PaymentError on a non-2xx response.
       */
      getPayment(idOrUrl: string): Promise<any>;
      /**
       * Resolve a HAL operation by rel and call its `href`. Accepts an
       * already-fetched payment-order payload, or an id/URL to GET first. Rel
       * matching is exact or by hyphen-suffix.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param rel The HAL operation rel to resolve.
       * @param body Optional request body for the operation.
       * @returns The operation's response.
       * @throws Error if the rel is not available on the payment order;
       *   PaymentError on a non-2xx response.
       */
      operation(paymentOrderOrUrl: string | Record<string, unknown>, rel: string, body?: unknown): Promise<any>;
      /**
       * Capture via the `capture` HAL operation.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param body The capture body.
       * @returns The capture result.
       * @throws Error if the `capture` rel is absent; PaymentError on non-2xx.
       */
      capturePayment(paymentOrderOrUrl: string | Record<string, unknown>, body: Record<string, unknown>): Promise<any>;
      /**
       * Refund via the `reversal` HAL operation.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param body The reversal body.
       * @returns The reversal result.
       * @throws Error if the `reversal` rel is absent; PaymentError on non-2xx.
       */
      refundPayment(paymentOrderOrUrl: string | Record<string, unknown>, body: Record<string, unknown>): Promise<any>;
      /**
       * Cancel via the `cancel` HAL operation.
       * @param paymentOrderOrUrl A fetched payment order, or an id/URL.
       * @param body Optional cancel body.
       * @returns The cancel result.
       * @throws Error if the `cancel` rel is absent; PaymentError on non-2xx.
       */
      cancelPayment(paymentOrderOrUrl: string | Record<string, unknown>, body?: Record<string, unknown>): Promise<any>;
    }
    /**
     * Build a SwedbankPay Checkout v2 client. Shares the v3 client + HAL model;
     * only the request payload shape differs. Auth: `Authorization: Bearer
     * <accessToken>`. Base URLs: test `api.externalintegration.payex.com`, prod
     * `api.payex.com`.
     * @param overrides Optional config; falls back to
     *   `SWEDBANKPAY_ACCESS_TOKEN`, `SWEDBANKPAY_MERCHANT_ID`,
     *   `SWEDBANKPAY_ENV`, `SWEDBANKPAY_BASE_URL`.
     * @returns A SwedbankPay v2 client.
     * @throws Error if `accessToken` / `merchantId` are neither passed nor in env.
     */
    function client(overrides?: SwedbankPayConfig): SwedbankPayClient;
  }
}
