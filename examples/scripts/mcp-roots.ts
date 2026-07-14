// mcp-roots.ts — mcp.serve() + ctx.roots / srv.onRootsChanged: a tool that
// asks the connected client for its filesystem roots, plus a persistent
// hook that fires whenever the client's root set changes.
//
// Like ctx.sample/ctx.elicit (mcp-sampling.ts / mcp-elicit.ts), ctx.roots
// round-trips through a real client (one that pre-seeds roots via
// client.AddRoots — see cmd/sercon/mcp_phase3_test.go's TestMCPRoots), which
// a plain HTTP POST can't do. NOT a `make demo` script: it serves over
// stdio and blocks until the peer disconnects.
//
// Driven by cmd/sercon/mcp_examples_test.go (TestMCPExamples_Roots), which
// runs this file as a subprocess, connects a real SDK client with roots
// pre-seeded via client.AddRoots, calls the "listRoots" tool, then adds
// another root post-connect to trigger onRootsChanged. Because stdout must
// stay pure JSON-RPC (see mcp-server-stdio.ts), the onRootsChanged callback
// below reports via runtime.log — which srv.stdio() redirects to stderr —
// and the test asserts against the captured stderr, the same technique
// TestMCPStdio uses for its two log lines.

const srv = mcp.serve({ name: "sercon-roots-demo", version: "1.0.0" });

srv.onRootsChanged((roots: any) => {
  const uris = roots.map((r: any) => r.uri).sort();
  runtime.log("roots changed:", JSON.stringify(uris));
});

srv.tool({
  name: "listRoots",
  description: "List the connected client's filesystem roots.",
  inputSchema: { type: "object" },
  async handler(_args: any, ctx: any) {
    const roots = await ctx.roots(); // Root[] : [{ uri, name? }, ...]
    const uris = roots.map((r: any) => r.uri).sort();
    return JSON.stringify(uris);
  },
});

await srv.stdio();
