// Type declarations for the sercon-bundled `paymentproviders` library.
// Hand-maintained (the library is an embedded module, not a reserved global).
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
    interface KcoClient {
      getPayment(id: string): Promise<any>;
      acknowledge(id: string): Promise<any>;
      capturePayment(id: string, input: CaptureInput): Promise<any>;
      refundPayment(id: string, input: RefundInput): Promise<any>;
      cancelPayment(id: string): Promise<any>;
      releaseRemainingAuthorization(id: string): Promise<any>;
      createCheckout(order: Record<string, unknown>): Promise<any>;
      getCheckout(id: string): Promise<any>;
    }
    /** Build a KCO v3 client. Reads KCO_MERCHANT_ID / KCO_SHARED_SECRET / KCO_ENV from the environment unless overridden. */
    function client(overrides?: KcoConfig): KcoClient;
  }
}
