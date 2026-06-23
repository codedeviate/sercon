import { buildClient, SwedbankPayConfig, SwedbankPayClient } from "../swedbankpay/common";

// v2 and v3 share the /psp/paymentorders create path + HAL model; the request
// payload shape (caller-supplied) is where they differ.
export function client(overrides: SwedbankPayConfig = {}): SwedbankPayClient {
  return buildClient(overrides, { version: "swedbankpayv2", createPath: "/psp/paymentorders" });
}
