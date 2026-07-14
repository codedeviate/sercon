package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// mcpServeHandleType is the object mcp.serve(...) resolves to — spliced
// verbatim into the "serve" MemberDoc's ReturnType (see googleHandleType /
// awsHandleType / azureHandleType in docs_cloud.go for the same pattern and
// its rationale). The handle is built at script-run time by mcpNamespace /
// (*mcpServer).handle in mcp.go — a Go `func(goja.FunctionCall) goja.Value`
// carries no static type information, so the d.ts emitter's reflection has
// nothing to recover here on its own; this constant is what actually reaches
// the emitted .d.ts. Must stay valid TypeScript on its own, and in lockstep
// with the six jsTool/jsResource/jsPrompt/jsStdio/jsListen/jsClose signatures
// in mcp_server.go.
const mcpServeHandleType = `{
  tool(spec: { name: string; description?: string; inputSchema: Record<string, unknown>; outputSchema?: Record<string, unknown>; handler(args: unknown, ctx: { requestId: string; clientInfo: { name: string; version: string } }): unknown | Promise<unknown> }): void;
  resource(spec: { uri: string; name: string; mimeType?: string; read(uri: string, ctx: { requestId: string; clientInfo: { name: string; version: string } }): unknown | Promise<unknown> }): void;
  prompt(spec: { name: string; description?: string; arguments?: Array<{ name: string; description?: string; required?: boolean }>; get(args: Record<string, string> | undefined, ctx: { requestId: string; clientInfo: { name: string; version: string } }): unknown | Promise<unknown> }): void;
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
			Summary: "Create an MCP (Model Context Protocol) server. Register zero or more tools/resources/prompts on the returned handle, then serve them over stdio() (Unix-only this phase) or listen() (Streamable HTTP, cross-platform). Built on the official modelcontextprotocol/go-sdk; only one transport may be started per handle, and registering a capability after a transport has started throws.",
			Params: []scriptengine.Param{
				{Name: "config", Type: "{ name: string; version: string; instructions?: string }", Desc: "name/version identify this server to clients during MCP's initialize handshake (surfaced to the client as serverInfo). instructions is an optional free-text hint about how to use this server (tone, expected workflow), surfaced to clients/LLMs during capability negotiation."},
			},
			ReturnType: mcpServeHandleType,
			Returns:    "A handle for registering capabilities and starting a transport: tool()/resource()/prompt() register handlers (only before a transport starts); stdio()/listen() start serving — mutually exclusive, starting a second transport on the same handle throws; close() is currently an inert placeholder (see serve.close).",
			Errors:     "Throws synchronously (not a rejected promise) if config is missing/not an object, or name/version is missing or empty.",
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
			Summary: "Register a tool: a named, schema-described callable that MCP clients (typically an LLM agent) can invoke. Must be called before the server starts serving (stdio()/listen()) — registering after start throws (adding tools to an already-serving connection needs a list-changed notification, a later phase).",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ name: string; description?: string; inputSchema: Record<string, unknown>; outputSchema?: Record<string, unknown>; handler(args: unknown, ctx: { requestId: string; clientInfo: { name: string; version: string } }): unknown | Promise<unknown> }", Desc: "name: unique tool name presented to clients. description: shown to the client/LLM to help it decide when to call this tool. inputSchema: a JSON Schema object describing the call arguments — passed through to the SDK as-is (not validated by sercon itself). outputSchema: optional JSON Schema describing structuredContent. handler(args, ctx): sync or async; may return a plain string (wrapped as a single text content item), an object shaped { content?, structuredContent?, isError? }, or throw/reject — a thrown or rejected handler is NOT a protocol error, it surfaces to the client as a tool result with isError: true."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the tool on the handle. Call multiple times to register multiple tools.",
			Errors:     "Throws synchronously if the server has already started serving, spec is not an object, name is missing/empty, or handler is not a function. A handler that throws or returns a rejected promise does not throw here — it surfaces per-call as an { isError: true } tool result seen by the client, not a synchronous or protocol-level error.",
			Example: `srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: { type: "object", properties: { a: { type: "number" }, b: { type: "number" } }, required: ["a", "b"] },
  async handler(args) { return String(args.a + args.b); },
});`,
		},
		"serve.resource": {
			Summary: "Register a resource: a URI-addressed piece of content clients can read (e.g. a file, a config blob, a generated report). Must be called before the server starts serving, for the same list-changed-notification reason as serve.tool.",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ uri: string; name: string; mimeType?: string; read(uri: string, ctx: { requestId: string; clientInfo: { name: string; version: string } }): unknown | Promise<unknown> }", Desc: "uri: the resource's identifier (any URI scheme, e.g. \"file:///report.txt\" or a custom scheme). name: a human-readable label shown when listing resources. mimeType: optional content-type hint. read(uri, ctx): sync or async; must return { text: string } or { blob: string | Uint8Array | ArrayBuffer } (blob accepts a base64 string or raw bytes) — any other shape, or a thrown/rejected handler, is a protocol-level error, unlike a tool handler's soft isError failure."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the resource on the handle.",
			Errors:     "Throws synchronously if the server has already started serving, spec is not an object, uri or name is missing/empty, or read is not a function. A read handler that throws, rejects, or returns a value without text or blob propagates as a resources/read protocol error to the client — there is no isError-style soft failure for resources.",
			Example: `srv.resource({
  uri: "config://app",
  name: "App config",
  mimeType: "application/json",
  read: async () => ({ text: JSON.stringify({ debug: true }) }),
});`,
		},
		"serve.prompt": {
			Summary: "Register a prompt: a named, parameterized template clients can fetch to seed a conversation. Must be called before the server starts serving, for the same list-changed-notification reason as serve.tool.",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ name: string; description?: string; arguments?: Array<{ name: string; description?: string; required?: boolean }>; get(args: Record<string, string> | undefined, ctx: { requestId: string; clientInfo: { name: string; version: string } }): unknown | Promise<unknown> }", Desc: "name: unique prompt name. description: shown to clients when listing prompts. arguments: the prompt's declared parameters (name/description/required), advertised so clients know what to supply. get(args, ctx): sync or async; must return { description?: string; messages: Array<{ role: string; content: unknown }> } — each message's content follows the same content-item shape as a tool result's content array (e.g. { type: \"text\", text }). Any other shape, or a thrown/rejected handler, is a protocol-level error."},
			},
			ReturnType: "void",
			Returns:    "Nothing — registers the prompt on the handle.",
			Errors:     "Throws synchronously if the server has already started serving, spec is not an object, name is missing/empty, arguments is present but not an array (or an entry is missing name), or get is not a function. A get handler that throws, rejects, or returns a malformed result propagates as a prompts/get protocol error to the client — there is no isError-style soft failure for prompts.",
			Example: `srv.prompt({
  name: "greet",
  description: "greet a user by name",
  arguments: [{ name: "user", required: true }],
  get: async (args) => ({
    messages: [{ role: "user", content: { type: "text", text: ` + "`Say hello to ${args.user}.`" + ` } }],
  }),
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
