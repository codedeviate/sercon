// mcp-sampling.ts — mcp.serve() + ctx.sample: a tool handler that asks the
// connected client's LLM to do work mid-request (the MCP "sampling"
// capability — server-initiated `sampling/createMessage`, not to be confused
// with a tool that itself calls an LLM provider directly).
//
// ctx.sample only resolves against a client that has advertised the
// sampling capability by wiring a CreateMessageHandler (see
// cmd/sercon/mcp_phase3_test.go's TestMCPSample) — there is no meaningful
// "self-test over a raw JSON-RPC POST" the way mcp-server-http.ts and its
// siblings do it, because a plain HTTP client can't answer the server's
// sampling/createMessage request that has to travel back down the very
// same connection mid-call. So, like mcp-server-stdio.ts, this is NOT a
// `make demo` script: it serves over stdio and blocks until the peer
// disconnects.
//
// Driven by cmd/sercon/mcp_examples_test.go (TestMCPExamples_Sampling),
// which builds the sercon binary, runs this file as a subprocess over
// mcp.CommandTransport, and connects the real SDK client with a canned
// CreateMessageHandler standing in for "the client's LLM".

const srv = mcp.serve({ name: "sercon-sampling-demo", version: "1.0.0" });

srv.tool({
  name: "summarize",
  description: "Ask the connected client's LLM to summarize the given text in one sentence.",
  inputSchema: {
    type: "object",
    properties: { text: { type: "string" } },
    required: ["text"],
  },
  async handler(args: any, ctx: any) {
    const r = await ctx.sample({
      messages: [{ role: "user", content: { type: "text", text: `Summarize in one sentence: ${args.text}` } }],
      maxTokens: 200,
      systemPrompt: "You are concise.",
    });
    // r => { content: { type, text }, model, stopReason, role }
    return JSON.stringify({ summary: r.content.text, model: r.model, stopReason: r.stopReason, role: r.role });
  },
});

await srv.stdio();
