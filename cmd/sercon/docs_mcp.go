package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// mcpCtxType is the `ctx` object every tool/resource/prompt handler receives
// as its 2nd argument. Shared verbatim across the tool/resource/prompt spec
// shapes below and mcpServeHandleType (kept as a Go constant, not
// interpolated, so every occurrence is visibly identical at a glance).
// progress()/log() were added in Phase 2 (Task 3, mcp_server.go's
// jsCtxProgress/jsCtxLog); requestId/clientInfo are unchanged from Phase 1.
// sample()/elicit()/roots() were added in Phase 3 (mcp_server.go's
// jsCtxSample/jsCtxElicit/jsCtxRoots) — each is a mid-handler server->client
// round trip (sampling/createMessage, elicitation/create, roots/list
// respectively) that only resolves against a client advertising the
// matching capability; all three reject with a clear "mcp: client does not
// support <capability>" error otherwise (checked against the negotiated
// ClientCapabilities before any SDK call is attempted). ctx is otherwise
// unchanged — the handler-arg shape stays (input, ctx).
const mcpCtxType = `{ requestId: string; clientInfo: { name: string; version: string }; progress(progress: number, total?: number): Promise<void>; log(level: string, message: string, data?: unknown): Promise<void>; sample(opts: { messages: Array<{ role: string; content: { type: string; text: string } }>; maxTokens?: number; systemPrompt?: string; temperature?: number; stopSequences?: string[]; includeContext?: string; modelPreferences?: { costPriority?: number; intelligencePriority?: number; speedPriority?: number } }): Promise<{ content: { type: string; text: string }; model: string; stopReason: string; role: string }>; elicit(opts: { message: string; schema: Record<string, unknown>; mode?: string }): Promise<{ action: "accept" | "decline" | "cancel"; content?: Record<string, unknown> }>; roots(): Promise<Array<{ uri: string; name?: string }>> }`

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
// jsOnRootsChanged/jsResourceUpdated/jsCompletion/jsStdio/jsListen/jsClose
// signatures in mcp.go/mcp_server.go. onRootsChanged (Phase 3, Task 4) and
// listen's `auth` option (Phase 3, Task 5, cmd/sercon/mcp_auth.go) are the
// two Phase-3 additions to this composite; everything else is unchanged from
// Phase 1/2.
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
  onRootsChanged(fn: (roots: Array<{ uri: string; name?: string }>) => void): void;
  resourceUpdated(uri: string): Promise<void>;
  completion(fn: (ref: { type: "prompt" | "resource"; name: string; uri: string }, argName: string, partial: string) => string[] | { values?: string[]; total?: number; hasMore?: boolean } | Promise<string[] | { values?: string[]; total?: number; hasMore?: boolean }> | null | undefined): void;
  stdio(): Promise<void>;
  listen(opts: { port: number; host?: string; path?: string; auth?: { verify(token: string, req: { method: string; path: string; header: Record<string, string> }): { subject?: string; scopes?: string[]; expiresAt?: number | string } | null | Promise<{ subject?: string; scopes?: string[]; expiresAt?: number | string } | null>; resourceMetadata?: { authorizationServers?: string[]; scopesSupported?: string[]; resourceName?: string; resourceDocumentation?: string; jwksUri?: string; bearerMethodsSupported?: string[]; resource?: string }; scopes?: string[] } }): Promise<{ url: string; stopped: Promise<void>; close(): Promise<void> }>;
  close(): void;
}`

// mcpClientHandleType is the object mcp.connect.stdio(...)/mcp.connect.http(...)
// resolve to — spliced verbatim into the "connect.stdio"/"connect.http"
// MemberDocs' ReturnType (Promise<mcpClientHandleType>), the same pattern
// mcpServeHandleType uses for the server side. Built at script-run time by
// (*mcpClient).handle in mcp_client.go — a Go `func(goja.FunctionCall)
// goja.Value` carries no static type information, so this constant is what
// actually reaches the emitted .d.ts. Must stay valid TypeScript on its own,
// and in lockstep with the obj.Set(...) calls in that function.
//
// Field shapes are simplified/generic reflections of the go-sdk's wire types
// (mcp.Tool, mcp.Resource, mcp.ResourceTemplate, mcp.Content,
// mcp.ResourceContents, mcp.Prompt/PromptArgument) — good enough for a
// script author to know what's there without pulling in the SDK's full
// JSON Schema machinery. `capabilities` and `inputSchema`/`outputSchema` are
// left as `Record<string, unknown>`/`unknown` rather than modeled, matching
// mcpCtxType's own precedent for JSON-Schema-shaped fields.
//
// Phase 2 (client) adds the ten methods below the Phase-1 CRUD methods:
// subscribe/unsubscribe/setLoggingLevel/complete (mid-connection calls, same
// asyncSettleResult shape as everything above) and the six onXxx notification
// setters (mc.cbMu-guarded single-slot registrations, last-writer-wins — see
// mcp_client.go's clientOptions/handle). Unlike the Phase-1 CRUD methods,
// these ten also get their own flat "connect.onToolsChanged" etc. MemberDoc
// entries below (mirroring mcp.serve's onSubscribe/onUnsubscribe/
// resourceUpdated/completion pattern) because their payload shapes and the
// setLoggingLevel-gates-onLoggingMessage relationship need more than this
// composite type's inline signatures convey.
//
// Phase 3 (client) adds the "host responder" surface: sercon now
// answers server->client requests instead of only ever being the one asking.
// Two of the three pieces (onSample/onElicit) are connect-time opts, not
// handle methods — they're documented as extra fields on connect.stdio's/
// connect.http's own "opts" Param below, not here. The third,
// setRoots(roots), IS a handle method (added below, mirroring the
// Phase-1/2 methods' inline-signature convention) — it updates the seeded
// `roots` connect opt at runtime via (*mcp.Client).AddRoots/RemoveRoots,
// firing roots/list_changed to the server. Gets its own flat
// "connect.setRoots" MemberDoc entry below for the same reason the ten
// Phase-2 methods do: its interaction with the seeded `roots` opt needs more
// than this composite type's inline signature conveys.
const mcpClientHandleType = `{
  serverInfo: { name: string; version: string; title?: string };
  capabilities: Record<string, unknown>;
  listTools(): Promise<Array<{ name: string; title?: string; description?: string; inputSchema: Record<string, unknown>; outputSchema?: Record<string, unknown> }>>;
  callTool(name: string, args?: Record<string, unknown>): Promise<{ content: Array<{ type: string; text?: string; data?: string; mimeType?: string; uri?: string; resource?: { uri: string; mimeType?: string; text?: string; blob?: string } }>; structuredContent?: unknown; isError: boolean }>;
  listResources(): Promise<Array<{ uri: string; name: string; title?: string; description?: string; mimeType?: string }>>;
  listResourceTemplates(): Promise<Array<{ uriTemplate: string; name: string; title?: string; description?: string; mimeType?: string }>>;
  readResource(uri: string): Promise<{ contents: Array<{ uri: string; mimeType?: string; text?: string; blob?: string }> }>;
  listPrompts(): Promise<Array<{ name: string; title?: string; description?: string; arguments?: Array<{ name: string; title?: string; description?: string; required: boolean }> }>>;
  getPrompt(name: string, args?: Record<string, string>): Promise<{ description?: string; messages: Array<{ role: string; content: { type: string; text?: string; data?: string; mimeType?: string; uri?: string } }> }>;
  ping(): Promise<void>;
  close(): Promise<void>;
  onToolsChanged(fn: () => void): void;
  onResourcesChanged(fn: () => void): void;
  onPromptsChanged(fn: () => void): void;
  onResourceUpdated(fn: (uri: string) => void): void;
  onLoggingMessage(fn: (message: { level: string; logger?: string; data: unknown }) => void): void;
  onProgress(fn: (progress: { progressToken: string | number; progress: number; total?: number; message?: string }) => void): void;
  subscribe(uri: string): Promise<void>;
  unsubscribe(uri: string): Promise<void>;
  setLoggingLevel(level: string): Promise<void>;
  complete(ref: { type: "prompt" | "resource"; name?: string; uri?: string }, argName: string, partial: string): Promise<{ values: string[]; total?: number; hasMore?: boolean }>;
  setRoots(roots: Array<{ uri: string; name?: string }>): void;
}`

// mcpSampleRequestType is the shape onSample's req argument takes — a
// simplified reflection of mcp.CreateMessageParams, structurally the same
// request the server side builds via jsCtxSample's opts parsing (mcp_server.go)
// just travelling in the opposite direction (server asking the client, here,
// rather than the script asking ctx.sample). Spliced into both the
// "connect.stdio"/"connect.http" opts Param docs and mcpHostOnSampleType below
// so the two stay in lockstep without hand-duplicating the field list.
const mcpSampleRequestType = `{ messages: Array<{ role: string; content: { type: string; text: string } }>; maxTokens?: number; systemPrompt?: string; temperature?: number; stopSequences?: string[]; includeContext?: string; modelPreferences?: { costPriority?: number; intelligencePriority?: number; speedPriority?: number } }`

// mcpSampleResultType is the shape onSample may return (sync or resolved from
// a Promise) — a plain string (the common case, wrapped as text content with
// model "sercon"/role "assistant"/stopReason "endTurn") or an object giving
// explicit control over content/model/stopReason/role. Mirrors
// toCreateMessageResult's two accepted shapes (mcp_client.go) exactly.
const mcpSampleResultType = `string | { content: { type: string; text: string } | string; model?: string; stopReason?: string; role?: string }`

// mcpHostOnSampleType is the full onSample connect-opt signature, spliced
// into the "connect.stdio"/"connect.http" opts Param Type strings below.
const mcpHostOnSampleType = `(req: ` + mcpSampleRequestType + `) => ` + mcpSampleResultType + ` | Promise<` + mcpSampleResultType + `>`

// mcpElicitRequestType is the shape onElicit's req argument takes — a
// simplified reflection of mcp.ElicitParams, the same request shape the
// server side builds via jsCtxElicit's opts parsing (mcp_server.go) travelling
// in the opposite direction.
const mcpElicitRequestType = `{ message: string; requestedSchema: Record<string, unknown>; mode?: string }`

// mcpElicitResultType is the shape onElicit must return (sync or resolved
// from a Promise) — mirrors toElicitResult's expected shape exactly
// (mcp_client.go): action is required, content is only meaningful when
// action is "accept".
const mcpElicitResultType = `{ action: "accept" | "decline" | "cancel"; content?: Record<string, unknown> }`

// mcpHostOnElicitType is the full onElicit connect-opt signature, spliced
// into the "connect.stdio"/"connect.http" opts Param Type strings below.
const mcpHostOnElicitType = `(req: ` + mcpElicitRequestType + `) => ` + mcpElicitResultType + ` | Promise<` + mcpElicitResultType + `>`

// mcpRootsOptType is the shape of the `roots` connect opt (and setRoots'
// single argument) — an array of { uri, name? }, matching mcp.Root and the
// server-facing Array<{uri, name?}> shape ctx.roots()/onRootsChanged already
// use in mcpCtxType/mcpServeHandleType above.
const mcpRootsOptType = `Array<{ uri: string; name?: string }>`

// mcpDocs documents the `mcp` global — mcp.serve(...) and every method of the
// handle it returns, plus mcp.connect.stdio/mcp.connect.http (the client
// side) and the handle they resolve to. Keys are relative to "mcp" (no
// "mcp." prefix — SetMemberDocsStructured prepends it), matching the
// convention in docs_server.go/docs_cloud.go.
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
//
// mcp.connect is the mirror-image case: unlike mcp.serve, it IS a real
// map[string]any (mcpConnectNamespace in mcp.go, nested under mcp's own
// namespace factory), so the surface walk DOES discover "connect" as a
// container node and "connect.stdio"/"connect.http" as real leaf members
// (see insertSurfaceMember in reference.go). "connect" itself needs no doc
// entry — same as "http"/"https"/"smtp" in docs_server.go, container nodes
// render with just a heading when undocumented — only the two leaf keys
// below need the full field set.
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
		"serve.onRootsChanged": {
			Summary: "Register a best-effort hook invoked whenever the connected client's filesystem/URI roots list changes (the MCP roots list-changed notification) — the persistent counterpart to ctx.roots()'s pull-based, per-request query. Only one onRootsChanged callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(roots: Array<{ uri: string; name?: string }>) => void", Desc: "invoked with the client's fresh root list (the same [{uri, name?}] shape ctx.roots() resolves to; name is \"\" when the client didn't set one). Its return value (and any thrown error or rejection) is ignored — the go-sdk's own notification handler has no way to fail the notification back to the client, so this hook is purely an observer."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onRootsChanged callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `srv.onRootsChanged((roots) => {
  runtime.log("roots changed:", JSON.stringify(roots.map((r) => r.uri)));
});

srv.tool({
  name: "listRoots",
  inputSchema: { type: "object" },
  async handler(_args, ctx) {
    const roots = await ctx.roots();
    return JSON.stringify(roots.map((r) => r.uri));
  },
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
			Example:    `srv.resourceUpdated("config://app");`,
		},
		"serve.completion": {
			Summary: "Register the handler invoked for a client's argument-autocompletion request (completion/complete) — suggesting values for a prompt argument or a resource-template URI variable as the user types. Only one completion callback is held at a time — a later call replaces the earlier registration. If never called, the server still advertises the completions capability and answers every request with an empty (\"no suggestions\") result rather than rejecting it.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(ref: { type: \"prompt\" | \"resource\"; name: string; uri: string }, argName: string, partial: string) => string[] | { values?: string[]; total?: number; hasMore?: boolean } | Promise<string[] | { values?: string[]; total?: number; hasMore?: boolean }> | null | undefined", Desc: "ref identifies what's being completed: type \"prompt\" (the prompt registered via serve.prompt, in name) or type \"resource\" (the resource-template URI registered via serve.resourceTemplate, in uri). Both name and uri are always set on ref — whichever one doesn't apply to the current type is set to the empty string \"\", never omitted or undefined — so discriminate on ref.type, not on checking a field for undefined. argName is the argument/variable name being completed and partial is the text typed so far. fn may return a plain string[] (the suggestions, in order), an object { values?, total?, hasMore? } for pagination hints, a Promise of either, or null/undefined/nothing to mean no suggestions."},
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
			Summary: "Serve this handle over the Streamable HTTP transport — a cross-platform, multi-client-capable alternative to stdio(): any number of clients can connect to a plain TCP/HTTP endpoint, rather than one client per subprocess. Optionally protect it as an OAuth 2.1 resource server via `opts.auth` — sercon validates bearer tokens and advertises where to obtain one; it never issues or registers tokens itself (that is the authorization server's job, out of scope here).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string; path?: string; auth?: { verify(token: string, req: { method: string; path: string; header: Record<string, string> }): { subject?: string; scopes?: string[]; expiresAt?: number | string } | null | Promise<{ subject?: string; scopes?: string[]; expiresAt?: number | string } | null>; resourceMetadata?: { authorizationServers?: string[]; scopesSupported?: string[]; resourceName?: string; resourceDocumentation?: string; jwksUri?: string; bearerMethodsSupported?: string[]; resource?: string }; scopes?: string[] } }", Desc: "port: the TCP port to bind (required). host: bind interface, defaults to \"127.0.0.1\". path: the HTTP path the MCP endpoint is mounted at, defaults to \"/mcp\". auth (optional): turns this listener into an OAuth 2.1 protected resource server (RFC 6750 bearer tokens + RFC 9728 protected-resource metadata) — omit it for the unauthenticated Phase-1/2 behavior. auth.verify(token, req) is called once per request with the bearer token string and a plain `{method, path, header}` request-info object (header values are already flattened to strings); return an identity object to accept the request (subject/scopes/expiresAt — expiresAt accepts a Unix-seconds number or an RFC 3339 string, and defaults to a 1-hour TTL if omitted) or null/undefined to reject it with a 401 (verify may be sync or return a Promise; a thrown or rejected verify is also treated as a 401, not a 500). auth.scopes lists the scopes enforced on every request (all must be present on the verified identity's scopes) and is also the WWW-Authenticate scope hint. auth.resourceMetadata fills the RFC 9728 metadata document served at `/.well-known/oauth-protected-resource` — authorizationServers is the list of AS issuer URLs clients should obtain tokens from (the whole point of a resource server: it never issues tokens itself); scopesSupported falls back to auth.scopes when omitted; resource (the RS's own identifier) defaults to the bound base URL when omitted. Dynamic client registration (DCR, RFC 7591) is intentionally out of scope — that's the authorization server's responsibility, not a resource server's."},
			},
			ReturnType: "Promise<{ url: string; stopped: Promise<void>; close(): Promise<void> }>",
			Returns:    "A promise that resolves as soon as the listener is bound (not when a client connects) to a handle: url is the full endpoint URL (e.g. \"http://127.0.0.1:38080/mcp\"); stopped resolves when the HTTP server stops (rejects on a non-close Serve error); close() begins a graceful shutdown and returns the same stopped promise.",
			Errors:     "Throws synchronously if a transport is already running on this handle, opts is missing/not an object, port is missing, or (with auth present) auth.verify is not a function. Throws (wrapping the bind error) if the listener fails to bind (e.g. address already in use) — a bind failure does NOT mark the handle as started, so listen() may be retried with a different port on the same handle. With auth configured, a request with a missing/invalid/expired/insufficiently-scoped bearer token never reaches your handlers — the middleware answers it directly with 401 and a WWW-Authenticate header pointing at the protected-resource metadata URL; this is enforced per-request at the HTTP layer, not surfaced as a script-side error.",
			Example: `const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });
srv.tool({ name: "ping", inputSchema: { type: "object" }, handler: () => "pong" });
const h = await srv.listen({ port: 38080 });
runtime.log("listening at", h.url);
// ... later
await h.close();

// With OAuth 2.1 resource-server protection:
const h2 = await srv.listen({
  port: 38081,
  auth: {
    verify: (token) => (token === "good-token" ? { subject: "demo-user", scopes: ["mcp"] } : null),
    resourceMetadata: { authorizationServers: ["https://auth.example.com"], scopesSupported: ["mcp"] },
    scopes: ["mcp"],
  },
});
await h2.close();`,
		},
		"serve.close": {
			Summary:    "Present on the handle for interface symmetry, but currently a no-op — it does not stop a running transport. To stop an HTTP listener, call the close() on the handle returned by listen(); a stdio server stops on its own once the peer disconnects (its stdio() promise settles then). A future phase may wire this into an explicit shutdown path.",
			ReturnType: "void",
			Returns:    "Nothing; calling it has no observable effect on a running transport.",
			Errors:     "Never throws.",
			Example: `const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });
srv.close(); // currently a no-op; use the listen() handle's close(), or let stdio() resolve on disconnect`,
		},
		"connect.stdio": {
			Summary: "Connect to an MCP server as a client, launching it as a subprocess and speaking newline-delimited JSON-RPC over its stdin/stdout — the shape most CLI-launched MCP servers (including sercon's own srv.stdio()) expect. Phase 2: consume an already-running server's tools/resources/prompts, react to its change notifications (onToolsChanged/onResourcesChanged/onPromptsChanged/onResourceUpdated), subscribe/unsubscribe to individual resources, opt into server logs (setLoggingLevel + onLoggingMessage), and request argument completions (complete) — see the ten onXxx/subscribe/unsubscribe/setLoggingLevel/complete entries below. Phase 3 adds the host responder surface: onSample/onElicit answer the server's own sampling/createMessage and elicitation/create requests (the client-side counterpart to a server tool's ctx.sample()/ctx.elicit()), and roots seeds the client's filesystem/URI roots (answering the server's roots/list) — see setRoots below for updating that set at runtime. Phase 4 rounds out the MCP client with connect.sse (the legacy SSE transport) and an OAuth client (connect.http's auth option) — neither applies to stdio itself (there's no HTTP handshake to attach a bearer token to, and this transport is a different wire format than SSE); Windows stdio support remains an unaddressed gap.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ command: string[]; env?: Record<string, string>; cwd?: string; onSample?: " + mcpHostOnSampleType + "; onElicit?: " + mcpHostOnElicitType + "; roots?: " + mcpRootsOptType + " }", Desc: "command: argv for the subprocess, e.g. [\"sercon\", \"server.ts\"] — command[0] is the executable (resolved via PATH), the rest are its arguments; must be a non-empty array. env: extra environment variables merged into the child's inherited environment (does not replace it). cwd: working directory for the child process; defaults to sercon's own cwd when omitted. onSample(req): answers the server's sampling/createMessage requests (ctx.sample() on the server side); req carries the same messages/maxTokens/systemPrompt/temperature/stopSequences/includeContext/modelPreferences fields ctx.sample's opts accepts. May return a plain string (wrapped as text content with model \"sercon\", role \"assistant\", stopReason \"endTurn\") or an object giving explicit control over content/model/stopReason/role; sync or async (a returned Promise is awaited). onElicit(req): answers the server's elicitation/create requests (ctx.elicit() on the server side); req carries { message, requestedSchema, mode? }. Must return { action: \"accept\"|\"decline\"|\"cancel\", content? } (content only meaningful on \"accept\"); sync or async. roots: seeds this client's filesystem/URI roots — an array of { uri, name? } — sent to the SDK via AddRoots before the connection is established, so the server's first roots/list sees them immediately rather than an empty set followed by a change notification. IMPORTANT: unlike onSample/onElicit (each advertised only when provided — see Errors), the roots capability itself is advertised by the underlying SDK unconditionally, regardless of whether this option is given; omitting roots simply means the initial root set is empty (still updatable later via setRoots)."},
			},
			ReturnType: "Promise<" + mcpClientHandleType + ">",
			Returns:    "A promise that resolves once the MCP initialize handshake completes to a session handle: serverInfo/capabilities reflect what the server advertised (capabilities.sampling/capabilities.elicitation here describe what the SERVER supports, not this client's own onSample/onElicit — there is no client-facing field reflecting what this client advertised), listTools/callTool/listResources/listResourceTemplates/readResource/listPrompts/getPrompt/ping/close drive the session, subscribe/unsubscribe/setLoggingLevel/complete make mid-connection requests, the six onXxx setters register server-push notification callbacks, and setRoots(roots) updates the seeded roots set at runtime. Holds the script's event loop open for the connection's lifetime (until close() or the subprocess exits) the same way an open server listener does.",
			Errors:     "Throws synchronously if opts is missing/not an object, command is missing/not a non-empty string array, onSample/onElicit are present but not functions, or a roots entry is missing a non-empty uri. The returned promise rejects if the subprocess fails to start or the initialize handshake fails (wrapped as \"mcp.connect: ...\"). Capability gating happens at connect time, not as a throw: the SDK client advertises the sampling capability if and only if onSample is provided (likewise elicitation/onElicit) — a server calling ctx.sample()/ctx.elicit() against a client that omitted the matching responder gets that server-side call rejected (\"mcp: client does not support sampling\"/\"...elicitation\"), not this connect call.",
			Example: `const c = await mcp.connect.stdio({ command: ["sercon", "server.ts"] });
runtime.log("connected to", c.serverInfo.name, c.serverInfo.version);
const r = await c.callTool("add", { a: 2, b: 3 });
runtime.log(r.content[0].text); // "5"
await c.close();

// As a host: answer the server's sampling/elicitation requests and seed roots.
const host = await mcp.connect.stdio({
  command: ["sercon", "server.ts"],
  onSample: (req) => "SUMMARY: " + req.messages[0].content.text,
  onElicit: (req) => ({ action: "accept", content: { confirmed: true } }),
  roots: [{ uri: "file:///workspace", name: "workspace" }],
});
await host.close();`,
		},
		"connect.http": {
			Summary: "Connect to an MCP server as a client over the Streamable HTTP transport — the cross-platform counterpart to connect.stdio, talking to a server already listening (e.g. one started with srv.listen(...)) instead of launching a subprocess. Same Phase 2/Phase 3 scope note as connect.stdio: change notifications, subscriptions, logging, and completion are supported (see the ten onXxx/subscribe/unsubscribe/setLoggingLevel/complete entries below), and onSample/onElicit/roots (see setRoots) let this connection act as a host for the server's own requests. Phase 4 (current) adds maxRetries (a reconnect cap for this transport) and auth (an OAuth 2.1 bearer-token client) — see below.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "the server's absolute MCP endpoint URL, e.g. \"http://127.0.0.1:38080/mcp\" (must be http or https with a host; anything else throws synchronously)."},
				{Name: "opts", Type: "{ headers?: Record<string, string>; maxRetries?: number; auth?: { getToken(): string | Promise<string> }; onSample?: " + mcpHostOnSampleType + "; onElicit?: " + mcpHostOnElicitType + "; roots?: " + mcpRootsOptType + " }", Optional: true, Desc: "optional. headers: extra HTTP headers sent with every request on this connection (e.g. a bearer token for a listen({auth}) protected server, as an alternative to auth below) — merged with sercon's default sercon-mcp/<version> User-Agent, which headers may override. maxRetries: caps how many times the Streamable-HTTP transport's own SSE-stream reconnect logic retries after a dropped connection; 0 or a negative number disables reconnection entirely (a single drop ends the session), omitted leaves the go-sdk's built-in default (5) in place. Streamable-HTTP-only — connect.sse's legacy SSE transport has no equivalent knob. auth: turns this connection into an OAuth 2.1 bearer-token client — the client-side counterpart to serve.listen's auth (resource-server) option. auth.getToken() is called once before the initial request and again whenever a request comes back 401/403, so the script can refresh an expired token; it may return a plain string or a Promise<string> and MUST resolve to a non-empty string — the returned value is sent verbatim as an `Authorization: Bearer <token>` header on every request. getToken runs on-loop (the same bridge tool/resource/prompt handlers use), so it may safely call other sercon bindings (e.g. net.http for a client-credentials token endpoint) or read a value cached from an earlier step. auth and headers.Authorization are independent — setting both means auth's bearer header is applied via the transport's own OAuth plumbing while headers still layers in anything else you supply (do not also set headers.Authorization yourself; the two would conflict). onSample(req): answers the server's sampling/createMessage requests (ctx.sample() on the server side); req carries messages/maxTokens/systemPrompt/temperature/stopSequences/includeContext/modelPreferences. May return a plain string (wrapped as text content, model \"sercon\", role \"assistant\", stopReason \"endTurn\") or an object giving explicit control over content/model/stopReason/role; sync or async. onElicit(req): answers the server's elicitation/create requests (ctx.elicit() on the server side); req carries { message, requestedSchema, mode? }. Must return { action: \"accept\"|\"decline\"|\"cancel\", content? }; sync or async. roots: seeds this client's filesystem/URI roots — an array of { uri, name? } — via AddRoots before the connection is established, so the server's first roots/list sees them immediately. IMPORTANT: the roots capability is advertised by the underlying SDK unconditionally regardless of whether this option is given (unlike onSample/onElicit, each advertised only when provided — see Errors); omitting roots just means the initial set is empty, still updatable later via setRoots."},
			},
			ReturnType: "Promise<" + mcpClientHandleType + ">",
			Returns:    "Same handle shape as connect.stdio: a promise that resolves once the initialize handshake completes to a session handle with serverInfo/capabilities, the listTools/callTool/listResources/listResourceTemplates/readResource/listPrompts/getPrompt/ping/close methods, the subscribe/unsubscribe/setLoggingLevel/complete mid-connection calls, the six onXxx notification setters, and setRoots(roots) to update the seeded roots set at runtime. Holds the script's event loop open for the connection's lifetime.",
			Errors:     "Throws synchronously if url is missing or not an absolute http(s) URL, maxRetries is present but not a number, auth is present but auth.getToken is not a function, onSample/onElicit are present but not functions, or a roots entry is missing a non-empty uri. The returned promise rejects if the HTTP connection or initialize handshake fails (wrapped as \"mcp.connect: ...\") — including a 401 from a listen({auth})-protected server when neither opts.headers nor opts.auth carries a valid bearer token, or if auth.getToken throws/rejects, resolves to a non-string, or resolves to an empty string (\"mcp.connect: auth.getToken must return a string\"/\"...a non-empty token string\"). Capability gating happens at connect time, not as a throw here: the SDK client advertises sampling/elicitation if and only if onSample/onElicit (respectively) is provided — a server calling ctx.sample()/ctx.elicit() against a client that omitted the matching responder gets that server-side call rejected, not this connect call.",
			Example: `const c = await mcp.connect.http("http://127.0.0.1:38080/mcp");
runtime.log("connected to", c.serverInfo.name);
const tools = await c.listTools();
runtime.log(tools.map(t => t.name).join(", "));
await c.ping();
await c.close();

// Against an OAuth-protected listener via a static header (see serve.listen's auth option):
const authed = await mcp.connect.http("http://127.0.0.1:38081/mcp", {
  headers: { Authorization: "Bearer good-token" },
});
await authed.close();

// Same, but via auth.getToken (re-invoked on 401/403 to refresh):
let cachedToken = "";
const oauthed = await mcp.connect.http("http://127.0.0.1:38081/mcp", {
  auth: {
    async getToken() {
      if (!cachedToken) cachedToken = "good-token"; // e.g. fetch via net.http client-credentials
      return cachedToken;
    },
  },
});
await oauthed.close();

// Disable the transport's automatic reconnect:
const noRetry = await mcp.connect.http("http://127.0.0.1:38080/mcp", { maxRetries: 0 });
await noRetry.close();

// As a host: answer the server's sampling/elicitation requests and seed roots.
const host = await mcp.connect.http("http://127.0.0.1:38080/mcp", {
  onSample: async (req) => {
    // wire this to services.ai to answer with a real LLM instead of an echo:
    // const r = await services.ai.send({ prompt: req.messages[0].content.text });
    // return r.output;
    return "SUMMARY: " + req.messages[0].content.text;
  },
  onElicit: (req) => ({ action: "accept", content: { confirmed: true } }),
  roots: [{ uri: "file:///workspace", name: "workspace" }],
});
host.setRoots([{ uri: "file:///workspace2", name: "workspace2" }]);
await host.close();`,
		},
		"connect.sse": {
			Summary: "Connect to an MCP server as a client over the legacy (2024-11-05) HTTP+SSE transport — for servers that predate Streamable HTTP (connect.http) and only expose the older two-endpoint SSE handshake. Routes through the same connectWith lifecycle as connect.stdio/connect.http, so everything else in Phase 1-4 works identically over this transport: the consume surface (listTools/callTool/etc.), the Phase-2 reactive surface (onToolsChanged/subscribe/setLoggingLevel/complete/...), and the Phase-3 host responder surface (onSample/onElicit/roots). The one Phase-4 addition that does NOT apply here is maxRetries: the go-sdk's SSE client transport has no reconnect-cap field to plumb (Streamable HTTP only) — see connect.http.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "the server's absolute SSE endpoint URL, e.g. \"http://127.0.0.1:38080/sse\" (must be http or https with a host; anything else throws synchronously)."},
				{Name: "opts", Type: "{ headers?: Record<string, string>; onSample?: " + mcpHostOnSampleType + "; onElicit?: " + mcpHostOnElicitType + "; roots?: " + mcpRootsOptType + " }", Optional: true, Desc: "optional — the same shape as connect.http's opts minus maxRetries/auth (neither applies to this transport: there's no reconnect cap to set, and no OAuth client wiring yet for SSE — use headers for a static bearer token instead). headers: extra HTTP headers sent with every request on this connection (e.g. a bearer token for a protected server) — merged with sercon's default sercon-mcp/<version> User-Agent, which headers may override. onSample(req)/onElicit(req)/roots: identical host-responder semantics to connect.http/connect.stdio — see connect.http's Params for the full field-by-field description of each."},
			},
			ReturnType: "Promise<" + mcpClientHandleType + ">",
			Returns:    "Same handle shape as connect.stdio/connect.http: a promise that resolves once the initialize handshake completes to a session handle with serverInfo/capabilities, the listTools/callTool/listResources/listResourceTemplates/readResource/listPrompts/getPrompt/ping/close methods, the subscribe/unsubscribe/setLoggingLevel/complete mid-connection calls, the six onXxx notification setters, and setRoots(roots) to update the seeded roots set at runtime. Holds the script's event loop open for the connection's lifetime.",
			Errors:     "Throws synchronously if url is missing or not an absolute http(s) URL, onSample/onElicit are present but not functions, or a roots entry is missing a non-empty uri. The returned promise rejects if the SSE handshake or initialize fails (wrapped as \"mcp.connect: ...\") — including a 401 from a protected server when opts.headers doesn't carry a valid bearer token (there is no opts.auth on this transport yet; retry/refresh it yourself before reconnecting). Capability gating happens at connect time, not as a throw here: the SDK client advertises sampling/elicitation if and only if onSample/onElicit (respectively) is provided — a server calling ctx.sample()/ctx.elicit() against a client that omitted the matching responder gets that server-side call rejected, not this connect call.",
			Example: `const c = await mcp.connect.sse("http://127.0.0.1:38080/sse");
runtime.log("connected to", c.serverInfo.name);
const tools = await c.listTools();
runtime.log(tools.map(t => t.name).join(", "));
await c.ping();
await c.close();

// Against a bearer-protected legacy SSE server (no opts.auth on this transport yet):
const authed = await mcp.connect.sse("http://127.0.0.1:38081/sse", {
  headers: { Authorization: "Bearer good-token" },
});
await authed.close();

// As a host: same onSample/onElicit/roots surface as connect.http/connect.stdio.
const host = await mcp.connect.sse("http://127.0.0.1:38080/sse", {
  onSample: (req) => "SUMMARY: " + req.messages[0].content.text,
  roots: [{ uri: "file:///workspace", name: "workspace" }],
});
await host.close();`,
		},

		// Phase 2 (client): the four mid-connection calls
		// (subscribe/unsubscribe/setLoggingLevel/complete) and six server-push
		// notification setters (onToolsChanged/onResourcesChanged/
		// onPromptsChanged/onResourceUpdated/onLoggingMessage/onProgress) on the
		// handle mcp.connect.stdio/mcp.connect.http resolve to. Keyed
		// "connect.X" — not a real "connect" surface member (connect only has
		// "stdio"/"http" statically; these are methods on the runtime handle
		// those calls resolve to) but an orphan doc key the reference generator
		// nests under "mcp.connect" anyway (buildNamespaceTree in
		// pkg/scriptengine/reference.go), the same trick mcp.serve's handle
		// methods use under "serve.X" one section up. Full field set required:
		// "mcp" is in sweptNamespaces.
		"connect.subscribe": {
			Summary: "Ask the server to notify this client whenever the given resource's content changes (resources/subscribe). The actual notification arrives via onResourceUpdated(fn) — call subscribe once per URI you care about, then register onResourceUpdated to react. Subscribing to a URI you're already subscribed to is a harmless no-op on most servers (a plain protocol round trip either way).",
			Params: []scriptengine.Param{
				{Name: "uri", Type: "string", Desc: "the resource URI to subscribe to — must match a URI the server actually tracks; subscribing to an unknown URI is a server-specific choice (some accept it silently, some reject it as a protocol error)."},
			},
			ReturnType: "Promise<void>",
			Returns:    "A promise that resolves once the server has acknowledged the subscription. Does not itself deliver any content — register onResourceUpdated(fn) beforehand to observe the resulting notifications.",
			Errors:     "Throws synchronously if uri is missing, null, or an empty string. The returned promise rejects if the connection is closed/dead or the server rejects the subscribe request as a protocol error (e.g. it doesn't support resource subscriptions at all).",
			Example: `const c = await mcp.connect.http("http://127.0.0.1:38080/mcp");
c.onResourceUpdated((uri) => { runtime.log("changed:", uri); });
await c.subscribe("cfg://app");
// ... some time later, once the server calls resourceUpdated("cfg://app") server-side ...
await c.unsubscribe("cfg://app");
await c.close();`,
		},
		"connect.unsubscribe": {
			Summary: "Ask the server to stop notifying this client about changes to the given resource (resources/unsubscribe) — the mirror of subscribe. Unsubscribing from a URI you were never subscribed to is a harmless no-op on most servers.",
			Params: []scriptengine.Param{
				{Name: "uri", Type: "string", Desc: "the resource URI to unsubscribe from — typically one previously passed to subscribe(uri)."},
			},
			ReturnType: "Promise<void>",
			Returns:    "A promise that resolves once the server has acknowledged the unsubscription. onResourceUpdated stops firing for this uri afterwards (assuming no other client-side subscription logic re-subscribes it).",
			Errors:     "Throws synchronously if uri is missing, null, or an empty string. The returned promise rejects if the connection is closed/dead or the server rejects the unsubscribe request as a protocol error.",
			Example: `await c.subscribe("cfg://app");
// ...
await c.unsubscribe("cfg://app");`,
		},
		"connect.setLoggingLevel": {
			Summary: "Opt this connection into receiving the server's log messages (logging/setLevel) at the given severity and higher. Per the MCP spec, a client receives nothing from a tool/resource/prompt handler's ctx.log(...) calls (see serve.tool's ctx) until it calls this — onLoggingMessage(fn) registered beforehand fires only after setLoggingLevel resolves, never before. Calling it again with a different level changes the threshold for all subsequent messages; there is no way to opt back out short of closing the connection.",
			Params: []scriptengine.Param{
				{Name: "level", Type: `"debug" | "info" | "notice" | "warning" | "error" | "critical" | "alert" | "emergency"`, Desc: "the minimum severity to receive, using the RFC 5424 syslog severity names the MCP spec re-uses (from least to most severe: debug, info, notice, warning, error, critical, alert, emergency) — the server sends this level and everything more severe. sercon does not validate the string against this list itself; an unrecognized value is passed through to the server as-is and its handling is server-specific."},
			},
			ReturnType: "Promise<void>",
			Returns:    "A promise that resolves once the server has acknowledged the level change. Does not itself deliver any messages — register onLoggingMessage(fn) beforehand (or at least before awaiting this) to observe the resulting notifications, since the server may start sending them immediately after acknowledging.",
			Errors:     "Throws synchronously if level is missing, null, or an empty string. The returned promise rejects if the connection is closed/dead or the server rejects the request as a protocol error (e.g. it doesn't support the logging capability at all).",
			Example: `const c = await mcp.connect.http("http://127.0.0.1:38080/mcp");
let lastLog: unknown = null;
c.onLoggingMessage((message) => { lastLog = message; });
await c.setLoggingLevel("info");
// now any ctx.log(...) call at "info" or more severe, on the server side,
// arrives here via onLoggingMessage as { level, logger?, data }
await c.close();`,
		},
		"connect.complete": {
			Summary: "Ask the server for argument-autocompletion suggestions (completion/complete) — the client-side counterpart to serve.completion's handler. Useful for building an interactive prompt-argument or resource-template-URI-variable picker against a remote server without guessing valid values yourself.",
			Params: []scriptengine.Param{
				{Name: "ref", Type: `{ type: "prompt" | "resource"; name?: string; uri?: string }`, Desc: "identifies what's being completed: type \"prompt\" completes an argument of a prompt the server registered (name: the prompt's name) or type \"resource\" completes a URI-template variable of a resource template the server registered (uri: the template's uriTemplate string, not a concrete URI). Set only the field matching type; the other is omitted."},
				{Name: "argName", Type: "string", Desc: "the argument name (for a prompt ref) or the URI template variable name (for a resource ref) being completed."},
				{Name: "partial", Type: "string", Desc: "the text typed so far; pass \"\" to ask for all suggestions with no filtering applied yet."},
			},
			ReturnType: "Promise<{ values: string[]; total?: number; hasMore?: boolean }>",
			Returns:    "A promise resolving to the server's suggestions: values is the ordered suggestion list (possibly empty — no match is not an error); total, when present, is the server's estimate of the full match count (may exceed values.length); hasMore, when present, signals more results exist beyond this page (the SDK does not expose a way to fetch the next page — a narrower partial is the only lever).",
			Errors:     "Throws synchronously if ref is missing/not an object, ref.type is neither \"prompt\" nor \"resource\", or argName is missing/empty. The returned promise rejects if the connection is closed/dead or the server rejects the request as a protocol error (e.g. it doesn't support the completions capability, or ref names something it never registered).",
			Example: `const c = await mcp.connect.stdio({ command: ["sercon", "server.ts"] });
const suggestions = await c.complete({ type: "prompt", name: "greet" }, "user", "ali");
runtime.log(suggestions.values.join(", ")); // e.g. "alice, alicia"
await c.close();`,
		},
		"connect.onToolsChanged": {
			Summary: "Register the callback invoked whenever the server's tool list changes (notifications/tools/list_changed) — e.g. a script connected to a server that itself calls srv.tool(...) at runtime after the initial connect. Only one onToolsChanged callback is held at a time — a later call replaces the earlier registration; there is no way to unregister short of closing the connection.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "() => void", Desc: "invoked with no arguments whenever the server announces its tool list changed — call listTools() again inside fn if you need the fresh list; the notification itself carries no payload. Its return value (and any thrown error or rejection) is ignored — the SDK's notification handler has no way to report it back to the server."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onToolsChanged callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `const c = await mcp.connect.http("http://127.0.0.1:38080/mcp");
c.onToolsChanged(async () => {
  const tools = await c.listTools();
  runtime.log("tools now:", tools.map(t => t.name).join(", "));
});`,
		},
		"connect.onResourcesChanged": {
			Summary: "Register the callback invoked whenever the server's resource list changes (notifications/resources/list_changed) — the mirror of onToolsChanged for resources. Only one onResourcesChanged callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "() => void", Desc: "invoked with no arguments whenever the server announces its resource list changed — call listResources() again inside fn if you need the fresh list. Its return value (and any thrown error or rejection) is ignored."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onResourcesChanged callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `c.onResourcesChanged(async () => {
  const resources = await c.listResources();
  runtime.log("resources now:", resources.map(r => r.uri).join(", "));
});`,
		},
		"connect.onPromptsChanged": {
			Summary: "Register the callback invoked whenever the server's prompt list changes (notifications/prompts/list_changed) — the mirror of onToolsChanged for prompts. Only one onPromptsChanged callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "() => void", Desc: "invoked with no arguments whenever the server announces its prompt list changed — call listPrompts() again inside fn if you need the fresh list. Its return value (and any thrown error or rejection) is ignored."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onPromptsChanged callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `c.onPromptsChanged(async () => {
  const prompts = await c.listPrompts();
  runtime.log("prompts now:", prompts.map(p => p.name).join(", "));
});`,
		},
		"connect.onResourceUpdated": {
			Summary: "Register the callback invoked whenever a subscribed resource's content changes (notifications/resources/updated) — the delivery half of the subscribe/unsubscribe pair (see connect.subscribe). Fires only for URIs this connection has subscribed to and only after the corresponding subscribe(uri) call has resolved. Only one onResourceUpdated callback is held at a time — a later call replaces the earlier registration, so a script juggling several subscribed URIs must dispatch on the uri argument itself, not register one callback per URI.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(uri: string) => void", Desc: "invoked with the URI whose content changed — typically followed by a readResource(uri) call to fetch the new content. Its return value (and any thrown error or rejection) is ignored."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onResourceUpdated callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `c.onResourceUpdated(async (uri) => {
  const doc = await c.readResource(uri);
  runtime.log("new content for", uri, ":", doc.contents[0].text);
});
await c.subscribe("cfg://app");`,
		},
		"connect.onLoggingMessage": {
			Summary: "Register the callback invoked for every server log message this connection has opted into (notifications/message) — see connect.setLoggingLevel, which this callback depends on. Registering onLoggingMessage alone, without ever calling setLoggingLevel, receives nothing: the server (per the MCP spec) does not send log messages until the client explicitly requests a level. Only one onLoggingMessage callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(message: { level: string; logger?: string; data: unknown }) => void", Desc: "invoked once per log message: level is the severity the server tagged it with (one of the same eight RFC 5424 names accepted by setLoggingLevel); logger, when the server set one, names the logging source; data is whatever JSON-serializable value the server's ctx.log(level, message, data?) call passed (commonly a string, but may be an object). Its return value (and any thrown error or rejection) is ignored."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onLoggingMessage callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `c.onLoggingMessage((message) => {
  runtime.log(` + "`[${message.level}]`" + `, message.logger ?? "server", message.data);
});
await c.setLoggingLevel("debug"); // required — otherwise onLoggingMessage never fires`,
		},
		"connect.onProgress": {
			Summary: "Register the callback invoked for server-sent progress updates (notifications/progress) correlated to an in-flight call this connection made (e.g. a long-running callTool) — the client-side counterpart to a server tool handler's ctx.progress(progress, total?). Only fires for calls where the server actually chose to send progress; not every tool call produces one. Only one onProgress callback is held at a time — a later call replaces the earlier registration.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "(progress: { progressToken: string | number; progress: number; total?: number; message?: string }) => void", Desc: "invoked once per progress notification: progressToken identifies which in-flight request this update belongs to (opaque — correlate it yourself if issuing multiple concurrent calls); progress is the cumulative amount of work done so far (server-defined units, monotonically increasing); total, when the server provided one, is the expected end value (progress/total gives a completion fraction); message, when present, is a short human-readable status string. Its return value (and any thrown error or rejection) is ignored."},
			},
			ReturnType: "void",
			Returns:    "Nothing — replaces any previously registered onProgress callback.",
			Errors:     "Throws synchronously (a TypeError) if fn is not a function.",
			Example: `c.onProgress((p) => {
  runtime.log(` + "`progress: ${p.progress}${p.total ? \"/\" + p.total : \"\"}`" + `, p.message ?? "");
});
await c.callTool("long-running-tool", {});`,
		},

		// Phase 3 (client): setRoots. Keyed "connect.setRoots" for the same
		// "orphan doc key nested under mcp.connect" reason the Phase-2 block
		// above documents — it's a method on the runtime handle
		// mcp.connect.stdio/mcp.connect.http resolve to, not a static member of
		// the "connect" object itself. Unlike onSample/onElicit/roots (which
		// are connect-time opts, documented as extra fields on connect.stdio's/
		// connect.http's own opts Param, not their own MemberDoc entries),
		// setRoots is a real handle method, so it gets the full-field entry
		// every other "connect.X" method here does.
		"connect.setRoots": {
			Summary: "Replace this connection's advertised filesystem/URI roots at runtime — the update counterpart to the `roots` connect opt's initial seed. Calls the SDK's RemoveRoots (for whatever this connection previously seeded/set) followed by AddRoots (for the new list), which together fire a roots/list_changed notification to the server; a server watching via srv.onRootsChanged(fn) sees the change pushed, and a server that only pulls via ctx.roots() sees the new set on its next call. Synchronous — there is no network round trip to await (AddRoots/RemoveRoots are pure in-memory bookkeeping on the client), so setRoots returns void, not a Promise, unlike almost every other method on this handle.",
			Params: []scriptengine.Param{
				{Name: "roots", Type: mcpRootsOptType, Desc: "the new complete root set — this REPLACES the previous set (whether it came from the connect opt's `roots` or an earlier setRoots call), it does not merge with it. Each entry needs a non-empty uri; name is an optional human-readable label. Pass an empty array to clear all roots."},
			},
			ReturnType: "void",
			Returns:    "Nothing. The server is not guaranteed to have observed the change by the time setRoots returns — only that the notification has been handed to the SDK client to send; a script that needs to be sure the server has picked up the new set should have the server confirm it back (e.g. via a tool call reading ctx.roots() after some delay, or by reacting to the server's own srv.onRootsChanged if scripting both sides).",
			Errors:     "Throws synchronously (a TypeError) if roots is not an array, or if any entry is not an object with a non-empty string uri. Never rejects — there is no Promise to reject.",
			Example: `const c = await mcp.connect.http("http://127.0.0.1:38080/mcp", {
  roots: [{ uri: "file:///a" }, { uri: "file:///b" }],
});
// ... later, the workspace changed:
c.setRoots([{ uri: "file:///c", name: "new workspace" }]);
await c.close();`,
		},
	}
}
