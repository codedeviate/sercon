// mcp-elicit.ts — mcp.serve() + ctx.elicit: a tool handler that pauses
// mid-request to ask the connected *user* (via the client, not the client's
// LLM — that's ctx.sample, see mcp-sampling.ts) to confirm an action or fill
// in a small form (the MCP "elicitation" capability).
//
// Like ctx.sample, ctx.elicit only resolves against a client that has
// advertised the capability by wiring an ElicitationHandler (see
// cmd/sercon/mcp_phase3_test.go's TestMCPElicit) — a plain HTTP POST can't
// answer the server's elicitation/create request that rides back down the
// same connection mid-call, so this is NOT a `make demo` script: it serves
// over stdio and blocks until the peer disconnects, mirroring
// mcp-sampling.ts / mcp-server-stdio.ts.
//
// Driven by cmd/sercon/mcp_examples_test.go (TestMCPExamples/Elicit), which
// runs this file as a subprocess and connects the real SDK client with a
// canned ElicitationHandler standing in for "the human at the keyboard".

const srv = mcp.serve({ name: "sercon-elicit-demo", version: "1.0.0" });

srv.tool({
  name: "deploy",
  description: "Deploy to the given target, after confirming with the user.",
  inputSchema: {
    type: "object",
    properties: { target: { type: "string" } },
    required: ["target"],
  },
  async handler(args: any, ctx: any) {
    const e = await ctx.elicit({
      message: `Confirm deploy to ${args.target}?`,
      schema: { type: "object", properties: { confirm: { type: "boolean" } } },
    });
    // e => { action: "accept"|"decline"|"cancel", content }  (content present on accept)
    if (e.action !== "accept" || !e.content || !e.content.confirm) {
      return JSON.stringify({ deployed: false, action: e.action });
    }
    return JSON.stringify({ deployed: true, target: args.target, action: e.action });
  },
});

await srv.stdio();
