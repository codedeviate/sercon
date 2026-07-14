// Demonstrates the resource-subscription surface: srv.onSubscribe /
// srv.onUnsubscribe (server-side hooks fired when a client subscribes to /
// unsubscribes from a resource URI) and srv.resourceUpdated (the call a
// script makes to tell the SDK "this resource changed, notify subscribers").
//
// LIMITATION, read before copying this pattern: the actual client-facing
// effect of resourceUpdated — a "notifications/resources/updated" push —
// rides on the same standalone SSE stream discussed in mcp-progress.ts's
// header comment, which this raw hand-rolled client never opens (a real
// client, e.g. the SDK's own — see cmd/sercon/mcp_phase2_test.go's
// TestMCPPhase2Subscribe* for the authoritative round trip using the real
// SDK client transport — keeps that stream open and receives the push
// normally). So instead of trying to observe the notification itself, this
// self-test verifies the parts that ARE visible over a plain request/
// response client:
//   - the "resources/subscribe" call succeeds, and the server's
//     onSubscribe hook has demonstrably fired by the time that call
//     returns (the go-sdk's SubscribeHandler — see mcp.go — awaits the JS
//     callback before responding, so this is a reliable, synchronous
//     signal, not a race);
//   - srv.resourceUpdated(uri) resolves without throwing, i.e. the script
//     side of "a resource changed" completes cleanly even with no client
//     actively watching it (the go-sdk's ResourceUpdated fans out to
//     current subscribers and does not itself error when delivery to any
//     one of them fails — see (*mcp.Server).ResourceUpdated);
//   - the same onSubscribe/onUnsubscribe round trip for
//     "resources/unsubscribe".

function parseSSE(body: string): any {
  const dataLine = body.split("\n").find((l) => l.startsWith("data:"));
  if (!dataLine) throw new Error("no SSE data frame in response: " + body);
  return JSON.parse(dataLine.slice(5).trim());
}

let nextId = 100;
async function rpc(url: string, headers: Record<string, string>, method: string, params?: any): Promise<any> {
  const res = await net.http.request("POST", url, {
    headers,
    body: JSON.stringify({ jsonrpc: "2.0", id: nextId++, method, params }),
  });
  runtime.assert.equal(res.status, 200, `${method} status`);
  return parseSSE(res.body);
}

const RESOURCE_URI = "counter://demo/value";

const port = 39040;
const srv = mcp.serve({ name: "sercon-subscribe-demo", version: "1.0.0" });

let counter = 0;
srv.resource({
  uri: RESOURCE_URI,
  name: "counter",
  mimeType: "text/plain",
  read: () => ({ text: String(counter) }),
});

// Recorded synchronously (see the file header comment on why this is safe
// to assert right after the corresponding RPC call returns).
const subscribed: string[] = [];
const unsubscribed: string[] = [];
srv.onSubscribe((uri: string) => { subscribed.push(uri); });
srv.onUnsubscribe((uri: string) => { unsubscribed.push(uri); });

const h = await srv.listen({ port });
runtime.log("listening at", h.url);

const jsonRPCHeaders = {
  "Content-Type": "application/json",
  Accept: "application/json, text/event-stream",
};

// 1) initialize
const initRes = await net.http.request("POST", h.url, {
  headers: jsonRPCHeaders,
  body: JSON.stringify({
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2025-06-18",
      capabilities: {},
      clientInfo: { name: "sercon-subscribe-client", version: "1.0.0" },
    },
  }),
});
runtime.assert.equal(initRes.status, 200, "initialize status");
const sessionId = initRes.headers["mcp-session-id"];
runtime.assert.ok(!!sessionId, "Mcp-Session-Id header present");

const sessionHeaders = { ...jsonRPCHeaders, "Mcp-Session-Id": sessionId };

// 2) notifications/initialized
const notifyRes = await net.http.request("POST", h.url, {
  headers: sessionHeaders,
  body: JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }),
});
runtime.assert.equal(notifyRes.status, 202, "notifications/initialized status");

// 3) resources/subscribe — by the time this returns, onSubscribe has fired.
const subMsg = await rpc(h.url, sessionHeaders, "resources/subscribe", { uri: RESOURCE_URI });
runtime.assert.ok(!subMsg.error, "resources/subscribe succeeded");
runtime.assert.equal(subscribed.join(","), RESOURCE_URI, "onSubscribe fired for the subscribed uri");

// 4) The resource "changes" and the script announces it. This resolves
// cleanly even though no client is actually listening for the push (see
// the file header comment) — that's the point of the assertion below.
counter = 42;
await srv.resourceUpdated(RESOURCE_URI);
runtime.log("resourceUpdated resolved without error (push itself isn't observable over this raw client)");

// 5) resources/unsubscribe — same synchronous-hook guarantee as subscribe.
const unsubMsg = await rpc(h.url, sessionHeaders, "resources/unsubscribe", { uri: RESOURCE_URI });
runtime.assert.ok(!unsubMsg.error, "resources/unsubscribe succeeded");
runtime.assert.equal(unsubscribed.join(","), RESOURCE_URI, "onUnsubscribe fired for the unsubscribed uri");

// Sanity: the resource itself still reads normally throughout.
const readMsg = await rpc(h.url, sessionHeaders, "resources/read", { uri: RESOURCE_URI });
runtime.assert.equal(readMsg.result.contents[0].text, "42", "resource reflects the updated value");

await h.close();
runtime.log("mcp-subscribe OK — closed");
