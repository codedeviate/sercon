export interface Operation { rel: string; href: string; method: string; }

// findOperation scans a SwedbankPay-style payload for an operation matching rel.
// Matches exact, or by suffix (endsWith) to tolerate qualified rels like
// "create-paymentorder-capture". Looks at payload.operations and
// payload.paymentOrder.operations.
export function findOperation(payload: any, rel: string): Operation | undefined {
  const ops: any[] =
    (payload && payload.operations) ||
    (payload && payload.paymentOrder && payload.paymentOrder.operations) ||
    [];
  for (const op of ops) {
    if (op && typeof op.rel === "string" && (op.rel === rel || op.rel.endsWith(rel))) {
      return { rel: op.rel, href: String(op.href), method: String(op.method || "POST") };
    }
  }
  return undefined;
}
