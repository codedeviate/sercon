import { buildClient, SwedbankPayConfig, SwedbankPayClient } from "../swedbankpay/common";

export function client(overrides: SwedbankPayConfig = {}): SwedbankPayClient {
  return buildClient(overrides, { version: "swedbankpayv3", createPath: "/psp/paymentorders" });
}
