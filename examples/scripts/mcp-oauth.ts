// mcp-oauth.ts — mcp.serve() + srv.listen({ auth }): a Streamable HTTP MCP
// server protected by OAuth 2.1 bearer-token verification, advertising RFC
// 9728 protected-resource metadata at
// /.well-known/oauth-protected-resource.
//
// This is NOT a `make demo` script (like mcp-server-stdio.ts /
// mcp-sampling.ts / mcp-elicit.ts / mcp-roots.ts): a real deployment's
// `verify` would call out to (or cryptographically check a token issued
// by) a real authorization server, which this offline demo doesn't have —
// here `verify` hardcodes one accepted bearer token to keep the example
// self-contained. Exercising it still needs a real HTTP client that can
// send (or withhold) that token, which is the "supporting client" this
// script needs.
//
// Lifecycle: unlike mcp-server-http.ts / mcp-resources.ts / mcp-progress.ts
// (which are their own client, self-test over a handful of requests, then
// close() and exit), this script binds a FIXED port and simply lets
// srv.listen's HoldRun keep the process alive after the top-level script
// body finishes — there's no self-close step, because the whole point is
// an external client driving it. cmd/sercon/mcp_examples_test.go
// (TestMCPExamples_OAuth) builds the sercon binary, runs this file as a
// subprocess, polls the fixed port until the listener accepts connections,
// then asserts:
//   - no token   -> 401, with WWW-Authenticate referencing the metadata URL
//   - bad token  -> 401
//   - good token -> MCP initialize + tools/call succeeds
//   - GET /.well-known/oauth-protected-resource -> 200 JSON metadata
// then kills the subprocess (there's no "done" signal to wait for — a real
// always-on OAuth-protected MCP server is stopped by its operator, not by
// itself).

const PORT = 39050; // fixed: this script never hands its handle to anything that could report an OS-assigned port back to a test

const srv = mcp.serve({ name: "sercon-oauth-demo", version: "1.0.0" });

srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: {
    type: "object",
    properties: { a: { type: "number" }, b: { type: "number" } },
    required: ["a", "b"],
  },
  async handler(args: any) {
    return String(args.a + args.b);
  },
});

const h = await srv.listen({
  port: PORT,
  auth: {
    // A real deployment would verify a signed JWT or call the authorization
    // server's introspection endpoint; this demo accepts exactly one
    // hardcoded token so the example stays offline and dependency-free.
    verify: (token: string, _req: unknown) =>
      token === "good-token" ? { subject: "demo-user", scopes: ["mcp"] } : null,
    resourceMetadata: {
      authorizationServers: ["https://auth.example.com"],
      scopesSupported: ["mcp"],
      resourceName: "sercon OAuth demo",
    },
    scopes: ["mcp"],
  },
});

runtime.log("mcp-oauth listening at", h.url, "(bearer token required: good-token)");

// No h.close() here on purpose — srv.listen's HoldRun keeps the process
// alive after this script body finishes running, so the external test
// client (or a curious human with `curl`) can still reach it. The test
// harness stops the process itself once it's done asserting.
