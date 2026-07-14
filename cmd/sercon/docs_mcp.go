package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// mcpCtxType is the `ctx` object every tool/resource/prompt handler receives
// as its 2nd argument. Shared verbatim across the tool/resource/prompt spec
// shapes below and mcpServeHandleType (kept as a Go constant, not
// interpolated, so every occurrence is visibly identical at a glance).
// progress()/log() were added in Phase 2 (Task 3, mcp_server.go's
// jsCtxProgress/jsCtxLog); requestId/clientInfo are unchanged from Phase 1.
const mcpCtxType = `{ requestId: string; clientInfo: { name: string; version: string }; progress(progress: number, total?: number): Promise<void>; log(level: string, message: string, data?: unknown): Promise<void> }`

// mcpServeHandleType is the object mcp.serve(...) resolves to — spliced
// verbatim into the "serve" MemberDoc's ReturnType (see googleHandleType /
// awsHandleType / azureHandleType in docs_cloud.go for the same pattern and
// its rationale). The handle is built at script-run time by mcpNamespace /
// (*mcpServer).handle in mcp.go — a Go `func(goja.FunctionCall) goja.Value`
// carries no static type information, so the d.ts emitter's reflection has
// nothing to recover here on its own; this constant is what actually reaches
// the emitted .d.ts. Must stay valid TypeScript on its own, and in lockstep
// with the jsTool/jsResource/jsResourceTemplate/jsPrompt/jsRemoveTool/
// jsRemoveResource/jsRemovePrompt/jsOnSubscribe/jsOnUnsubscribe/
// jsResourceUpdated/jsCompletion/jsStdio/jsListen/jsClose signatures in
// mcp.go/mcp_server.go.
const mcpServeHandleType = `{
  tool(spec: { name: string; description?: string; inputSchema: Record<string, unknown>; outputSchema?: Record<string, unknown>; handler(args: unknown, ctx: ` + mcpCtxType + `): unknown | Promise<unknown> }): void;
  resource(spec: { uri: string; name: string; mimeType?: string; read(uri: string, ctx: ` + mcpCtxType + `): unknown | Promise<unknown> }): void;
  resourceTemplate(spec: { uriTemplate: string; name: string; mimeType?: string; read(uri: string, ctx: ` + mcpCtxType + `): unknown | Promise<unknown> }): void;
  prompt(spec: { name: string; description?: string; arguments?: Array<{ name: string; description?: string; required?: boolean }>; get(args: Record<string, string> | undefined, ctx: ` + mcpCtxType + `): unknown | Promise<unknown> }): void;
  removeTool(name: string): void;
  removeResource(uri: string): void;
  removePrompt(name: string): void;
  onSubscribe(fn: (uri: string) => void): void;
  onUnsubscribe(fn: (uri: string) => void): void;
  resourceUpdated(uri: string): Promise<void>;
  completion(fn: (ref: { type: "prompt" | "resource"; name: string; uri: string }, argName: string, partial: string) => string[] | { values?: string[]; total?: number; hasMore?: boolean } | Promise<string[] | { values?: string[]; total?: number; hasMore?: boolean }> | null | undefined): void;
  stdio(): Promise<void>;
  listen(opts: { port: number; host?: string; path?: string }): Promise<{ url: string; stopped: Promise<void>; close(): Promise<void> }>;
  close(): void;
}`

// mcpDocs documents the `mcp` global — mcp.serve(...) and every method of the
// handle it returns. Keys are relative to "mcp" (no "mcp." prefix —
// SetMemberDocsStructured prepends it), matching the convention in
// docs_server.go/docs_cloud.go.
//
// Unlike cloud's runtime-built provider handles (google()/aws()/azure()),
// mcp.serve(...) has no further nesting below its own methods — there is no
// "serve.tool"-shaped container entry the way "google.storage" is a
// container above "google.storage.listBuckets". Every key below is therefore
// fully populated (Summary/Params/ReturnType/Returns/Errors/Example): this
// namespace is in sweptNamespaces (docs_completeness_test.go), so
// TestDocsComplete requires every entry here to meet the full standard, not
// just the walked "serve" entry the brief calls out explicitly.
//
// The reference generator (pkg/scriptengine/reference.go) renders these flat
// "serve.*" keys as children of the "serve" node via its doc-key merge
// (buildNamespaceTree): mcp.serve is the only surface member the walk can see
// (a func, not a map[string]any), so "serve.tool" etc. reach the MANUAL §17
// output purely through this doc map, the same mechanism cloud's per-service
// method entries use.
func mcpDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"serve": {
			Summary: "Create an MCP (Model Context Protocol) server. Register zero or more tools/resources/prompts/resource templates on the returned handle — at any time, including after a transport has started, since registering them fires a list-changed notification to already-connected clients — then serve them over stdio() (Unix-only this phase) or listen() (Streamable HTTP, cross-platform). Built on the official modelcontextprotocol/go-sdk; only one transport may be started per handle — starting a second one throws.",
			Params: []scriptengine.Param{
				{Name: "config", Type: "{ name: string; version: string; instructions?: string; pageSize?: number }", Desc: "name/version identify this server to clients during MCP's initialize handshake (surfaced to the client as serverInfo). instructions is an optional free-text hint about how to use this server (tone, expected workflow), surfaced to clients/LLMs during capability negotiation. pageSize is an optional positive integer capping how many tools/resources/prompts/resource templates a single list response returns before the client must page with a cursor; defaults to the SDK's built-in page size (1000) when omitted. Present-but-invalid (zero, negative, or non-integer) throws synchronously."},
			},
			ReturnType: mcpServeHandleType,
			Returns:    "A handle for registering capabilities and starting a transport: tool()/resource()/resourceTemplate()/prompt() register handlers and removeTool()/removeResource()/removePrompt() unregister them — all callable at any time; onSubscribe()/onUnsubscribe() register resource-subscription hooks and resourceUpdated() notifies subscribers; completion() registers an argument-autocompletion handler; stdio()/listen() start serving — mutually exclusive, starting a second transport on the same handle throws; close() is currently an inert placeholder (see serve.close).",
			Errors:     "Throws synchronously if config is missing/not an object, name/version is missing or empty, or pageSize is present but not a positive integer.",
			Example: `const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });
srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: { type: "object", properties: { a: { type: "number" }, b: { type: "number" } }, required: ["a", "b"] },
  async handler(args) { return String(args.a + args.b); },
});
await srv.stdio();`,
		},
		"serve.tool": {
			Summary: "Register a tool: a named, schema-described callable that MCP clients (typically an LLM agent) can invoke. Callable at any time, including after a transport has started — the SDK fires a tools/list_changed notification to already-connected clients when a tool is added post-connect, so a script can grow its tool set at runtime (e.g. from within another handler).",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ name: string; description?: string; inputSchema: Record<string, unknown>; outputSchema?: Record<string, unknown>; handler(args: unknown, ctx: " + mcpCtxType + "): unknown | Promise<unknown> }", Desc: "name: unique tool name presented to clients. description: shown to the client/LLM to help it decide when to call this tool. inputSchema: a JSON Schema object describing the call arguments — passed through to the SDK as-is (not validated by sercon itself). outputSchema: optional JSON Schema describing structuredContent. handler(args, ctx): sync or async; may return a plain string (wrapped as a single text content item), an object shaped { content?, structuredContent?, isError? }, or throw/reject — a thrown or rejected handler is NOT a protocol error, it surfaces to the client as a tool result with isError: true. ctx adds progress(progress, total?) and log(level, message, data?) to the requestId/clientInfo pair — see §5.15.1's progress/logging concepts for the SetLoggingLevel caveat on log()."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the tool on the handle. Call multiple times to register multiple tools; call srv.removeTool(name) to unregister one.",
			Errors:     "Throws synchronously if spec is not an object, name is missing/empty, or handler is not a function. A handler that throws or returns a rejected promise does not throw here — it surfaces per-call as an { isError: true } tool result seen by the client, not a synchronous or protocol-level error.",
			Example: `srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: { type: "object", properties: { a: { type: "number" }, b: { type: "number" } }, required: ["a", "b"] },
  async handler(args) { return String(args.a + args.b); },
});`,
		},
		"serve.resource": {
			Summary: "Register a resource: a URI-addressed piece of content clients can read (e.g. a file, a config blob, a generated report). Callable at any time, including after a transport has started, for the same list-changed-notification reason as serve.tool.",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ uri: string; name: string; mimeType?: string; read(uri: string, ctx: " + mcpCtxType + "): unknown | Promise<unknown> }", Desc: "uri: the resource's identifier (any URI scheme, e.g. \"file:///report.txt\" or a custom scheme). name: a human-readable label shown when listing resources. mimeType: optional content-type hint. read(uri, ctx): sync or async; must return { text: string } or { blob: string | Uint8Array | ArrayBuffer } (blob accepts a base64 string or raw bytes) — any other shape, or a thrown/rejected handler, is a protocol-level error, unlike a tool handler's soft isError failure. ctx is the same shape as a tool handler's (see serve.tool)."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the resource on the handle. Call srv.removeResource(uri) to unregister it.",
			Errors:     "Throws synchronously if spec is not an object, uri or name is missing/empty, or read is not a function. A read handler that throws, rejects, or returns a value without text or blob propagates as a resources/read protocol error to the client — there is no isError-style soft failure for resources.",
			Example: `srv.resource({
  uri: "config://app",
  name: "App config",
  mimeType: "application/json",
  read: async () => ({ text: JSON.stringify({ debug: true }) }),
});`,
		},
		"serve.resourceTemplate": {
			Summary: "Register a resource template: an RFC 6570 URI template (e.g. \"db:///{table}/{id}\") describing a family of resources a client can read by supplying a concrete URI that matches the pattern, rather than one fixed uri the way serve.resource registers. Callable at any time, including after a transport has started, for the same list-changed-notification reason as serve.tool.",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ uriTemplate: string; name: string; mimeType?: string; read(uri: string, ctx: " + mcpCtxType + "): unknown | Promise<unknown> }", Desc: "uriTemplate: the RFC 6570 template string advertised to clients (e.g. \"db:///{table}/{id}\"). name: a human-readable label shown when listing resource templates. mimeType: optional content-type hint. read(uri, ctx): sync or async, invoked with the concrete URI the client actually requested (e.g. \"db:///users/42\") — not the template string; must return { text: string } or { blob: string | Uint8Array | ArrayBuffer }, same contract as serve.resource's read. ctx is the same shape as a tool handler's (see serve.tool)."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the resource template on the handle.",
			Errors:     "Throws synchronously if spec is not an object, uriTemplate or name is missing/empty, or read is not a function. A read handler that throws, rejects, or returns a value without text or blob propagates as a resources/read protocol error to the client, same as serve.resource.",
			Example: `srv.resourceTemplate({
  uriTemplate: "db:///{table}/{id}",
  name: "row",
  mimeType: "application/json",
  read: (uri) => ({ text: "row at " + uri }),
});
// a client reading "db:///users/42" invokes read("db:///users/42", ctx)`,
		},
		"serve.prompt": {
			Summary: "Register a prompt: a named, parameterized template clients can fetch to seed a conversation. Callable at any time, including after a transport has started, for the same list-changed-notification reason as serve.tool.",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ name: string; description?: string; arguments?: Array<{ name: string; description?: string; required?: boolean }>; get(args: Record<string, string> | undefined, ctx: " + mcpCtxType + "): unknown | Promise<unknown> }", Desc: "name: unique prompt name. description: shown to clients when listing prompts. arguments: the prompt's declared parameters (name/description/required), advertised so clients know what to supply. get(args, ctx): sync or async; must return { description?: string; messages: Array<{ role: string; content: unknown }> } — each message's content follows the same content-item shape as a tool result's content array (e.g. { type: \"text\", text }). Any other shape, or a thrown/rejected handler, is a protocol-level error. ctx is the same shape as a tool handler's (see serve.tool)."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the prompt on the handle. Call srv.removePrompt(name) to unregister it.",
			Errors:     "Throws synchronously if spec is not an object, name is missing/empty, arguments is present but not an array (or an entry is missing name), or get is not a function. A get handler that throws, rejects, or returns a malformed result propagates as a prompts/get protocol error to the client — there is no isError-style soft failure for prompts.",
			Example: `srv.prompt({
  name: "greet",
  description: "greet a user by name",
  arguments: [{ name: "user", required: true }],
  get: async (args) => ({
    messages: [{ role: "user", content: { type: "text", text: ` + "`Say hello to ${args.user}.`" + ` } }],
  }),
});`,
		},
		"serve.removeTool": {
			Summary: "Unregister a previously added tool by name. Callable at any time — before or after a transport has started; removing a name post-connect fires a tools/list_changed notification to connected clients. Removing a name that was never registered (or already removed) is a silent no-op.",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "the tool name passed to serve.tool's spec.name."},
			},
			ReturnType: "void",
			Returns:    "Nothing.",
			Errors:     "Throws synchronously (a TypeError) if name is missing, null, or an empty string.",
			Example: `srv.tool({ name: "temp", inputSchema: { type: "object" }, handler: () => "hi" });
// ... later, once the capability is no longer relevant:
srv.removeTool("temp");`,
		},
		"serve.removeResource": {
			Summary: "Unregister a previously added resource by URI. Callable at any time — before or after a transport has started; removing a URI post-connect fires a resources/list_changed notification to connected clients. Removing a URI that was never registered (or already removed) is a silent no-op.",
			Params: []scriptengine.Param{
				{Name: "uri", Type: "string", Desc: "the resource URI passed to serve.resource's spec.uri. There is no equivalent remove method for a resource template registered via serve.resourceTemplate — that's a currently-unbound gap in the script surface, not a missing SDK capability."},
			},
			ReturnType: "void",
			Returns:    "Nothing.",
			Errors:     "Throws synchronously (a TypeError) if uri is missing, null, or an empty string.",
			Example: `srv.resource({ uri: "config://temp", name: "temp", read: () => ({ text: "{}" }) });
// ... later:
srv.removeResource("config://temp");`,
		},
		"serve.removePrompt": {
			Summary: "Unregister a previously added prompt by name. Callable at any time — before or after a transport has started; removing a name post-connect fires a prompts/list_changed notification to connected clients. Removing a name that was never registered (or already removed) is a silent no-op.",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "the prompt name passed to serve.prompt's spec.name."},
			},
			ReturnType: "void",
			Returns:    "Nothing.",
			Errors:     "Throws synchronously (a TypeError) if name is missing, null, or an empty string.",
			Example: `srv.prompt({ name: "temp", get: () => ({ messages: [] }) });
// ... later:
srv.removePrompt("temp");`,
		},
		"serve.onSubscribe": {
			Summary: "Register a best-effort hook invoked whenever a client subscribes to a resource (resources/subscribe). Typically used to start watching a resource's backing data (a file, a DB row, …) only when a client actually cares, pairing with serve.onUnsubscribe to stop watching and serve.resourceUpdated to notify once it changes. Only one onSubscribe callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(uri: string) => void", Desc: "invoked with the URI the client subscribed to. Its return value (and any thrown error or rejection) is ignored — the subscribe request always succeeds from the client's point of view; this hook cannot fail it."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onSubscribe callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `const watchers = new Map<string, () => void>();
srv.onSubscribe((uri) => {
  runtime.log("client subscribed to", uri);
  // start watching uri's backing data here
});
srv.onUnsubscribe((uri) => {
  runtime.log("client unsubscribed from", uri);
  // stop watching uri's backing data here
});`,
		},
		"serve.onUnsubscribe": {
			Summary: "Register a best-effort hook invoked whenever a client unsubscribes from a resource (resources/unsubscribe) — the mirror of serve.onSubscribe. Only one onUnsubscribe callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(uri: string) => void", Desc: "invoked with the URI the client unsubscribed from. Its return value (and any thrown error or rejection) is ignored — the unsubscribe request always succeeds from the client's point of view; this hook cannot fail it."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onUnsubscribe callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `srv.onUnsubscribe((uri) => {
  runtime.log("no longer watching", uri);
});`,
		},
		"serve.resourceUpdated": {
			Summary: "Notify every client currently subscribed to uri that the resource's content has changed (resources/updated). This is the script's half of the subscription pair: the SDK tracks the actual subscriber set itself (via the subscribe/unsubscribe plumbing backing serve.onSubscribe/onUnsubscribe) and fans this notification out to whoever is subscribed to uri right now — calling it for a URI with no subscribers is a harmless no-op. Callable at any time after mcp.serve(), including before any transport has started.",
			Params: []scriptengine.Param{
				{Name: "uri", Type: "string", Desc: "the resource URI that changed — must match a URI a client may have subscribed to (does not need to be a URI you've registered with serve.resource; it's just an opaque identifier from the notification's point of view)."},
			},
			ReturnType: "Promise<void>",
			Returns:    "A promise that resolves once the notification has been sent (or immediately if there is no active transport to send it over).",
			Errors:     "Throws synchronously (a TypeError) if uri is missing, null, or an empty string. The returned promise rejects if the underlying notification send fails.",
			Example: `srv.resourceUpdated("config://app");`,
		},
		"serve.completion": {
			Summary: "Register the handler invoked for a client's argument-autocompletion request (completion/complete) — suggesting values for a prompt argument or a resource-template URI variable as the user types. Only one completion callback is held at a time — a later call replaces the earlier registration. If never called, the server still advertises the completions capability and answers every request with an empty (\"no suggestions\") result rather than rejecting it.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(ref: { type: \"prompt\" | \"resource\"; name?: string; uri?: string }, argName: string, partial: string) => string[] | { values?: string[]; total?: number; hasMore?: boolean } | Promise<string[] | { values?: string[]; total?: number; hasMore?: boolean }> | null | undefined", Desc: "ref identifies what's being completed: type \"prompt\" with name set (the prompt registered via serve.prompt) or type \"resource\" with uri set (the resource-template URI registered via serve.resourceTemplate). argName is the argument/variable name being completed and partial is the text typed so far. fn may return a plain string[] (the suggestions, in order), an object { values?, total?, hasMore? } for pagination hints, a Promise of either, or null/undefined/nothing to mean no suggestions."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered completion callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function. Unlike a tool handler, a completion handler that throws or rejects propagates as a real completion/complete protocol error to the client — there is no isError-style soft failure for completion.",
			Example: `srv.prompt({
  name: "greet",
  arguments: [{ name: "user", required: true }],
  get: async (args) => ({ messages: [{ role: "user", content: { type: "text", text: "hi " + args.user } }] }),
});

const users = ["alice", "alicia", "bob"];
srv.completion((ref, argName, partial) => {
  if (ref.type === "prompt" && ref.name === "greet" && argName === "user") {
    return users.filter((u) => u.startsWith(partial));
  }
  return [];
});`,
		},
		"serve.stdio": {
			Summary:    "Serve this handle over stdio (newline-delimited JSON-RPC on stdin/stdout) — the transport clients like Claude Desktop use when they launch this script as a subprocess. Unix-only this phase: on Windows the returned promise rejects immediately with a clear error (stdout can't be safely separated from the JSON-RPC stream there) — use listen() instead. sercon's own output (console.*, runtime.log) is transparently redirected to stderr starting the moment mcp.serve() is called (not just once stdio() begins), so stdout carries only protocol frames for the lifetime of the connection.",
			ReturnType: "Promise<void>",
			Returns:    "A promise that resolves once the client disconnects (stdin closes / the session ends) and rejects if the transport fails to start (e.g. on Windows) or the JSON-RPC session ends with an error.",
			Errors:     "Throws synchronously if a transport is already running on this handle (stdio()/listen() already called). The returned promise rejects on Windows with \"mcp: stdio() is not supported on windows (console output cannot be separated from the JSON-RPC stream); use listen() instead\", or if the underlying session ends abnormally.",
			Example: `const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });
srv.tool({ name: "ping", inputSchema: { type: "object" }, handler: () => "pong" });
await srv.stdio();`,
		},
		"serve.listen": {
			Summary: "Serve this handle over the Streamable HTTP transport — a cross-platform, multi-client-capable alternative to stdio(): any number of clients can connect to a plain TCP/HTTP endpoint, rather than one client per subprocess.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string; path?: string }", Desc: "port: the TCP port to bind (required). host: bind interface, defaults to \"127.0.0.1\". path: the HTTP path the MCP endpoint is mounted at, defaults to \"/mcp\"."},
			},
			ReturnType: "Promise<{ url: string; stopped: Promise<void>; close(): Promise<void> }>",
			Returns:    "A promise that resolves as soon as the listener is bound (not when a client connects) to a handle: url is the full endpoint URL (e.g. \"http://127.0.0.1:38080/mcp\"); stopped resolves when the HTTP server stops (rejects on a non-close Serve error); close() begins a graceful shutdown and returns the same stopped promise.",
			Errors:     "Throws synchronously if a transport is already running on this handle, opts is missing/not an object, or port is missing. Throws (wrapping the bind error) if the listener fails to bind (e.g. address already in use) — a bind failure does NOT mark the handle as started, so listen() may be retried with a different port on the same handle.",
			Example: `const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });
srv.tool({ name: "ping", inputSchema: { type: "object" }, handler: () => "pong" });
const h = await srv.listen({ port: 38080 });
runtime.log("listening at", h.url);
// ... later
await h.close();`,
		},
		"serve.close": {
			Summary:    "Present on the handle for interface symmetry, but currently a no-op — it does not stop a running transport. To stop an HTTP listener, call the close() on the handle returned by listen(); a stdio server stops on its own once the peer disconnects (its stdio() promise settles then). A future phase may wire this into an explicit shutdown path.",
			ReturnType: "void",
			Returns:    "Nothing; calling it has no observable effect on a running transport.",
			Errors:     "Never throws.",
			Example: `const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });
srv.close(); // currently a no-op; use the listen() handle's close(), or let stdio() resolve on disconnect`,
		},
	}
}
