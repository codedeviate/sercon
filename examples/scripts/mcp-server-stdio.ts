// mcp-server-stdio.ts — an MCP server served over stdio.
//
// stdout carries ONLY newline-delimited JSON-RPC; sercon's own output
// (console.log / runtime.log) is redirected to stderr by srv.stdio() so it
// can't corrupt the protocol stream. The two log lines below run BEFORE the
// server starts specifically to exercise that guarantee — a client on the
// other end must still parse stdout cleanly.
//
// This is not a `make demo` script: it's a stdio server driven by the Go
// integration test (cmd/sercon/mcp_stdio_test.go), like hang.ts is driven
// separately. It blocks on stdin and only exits when the peer disconnects.

const srv = mcp.serve({ name: "sercon-stdio-demo", version: "1.0.0" });

srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: {
    type: "object",
    properties: { a: { type: "number" }, b: { type: "number" } },
    required: ["a", "b"],
  },
  async handler(args) {
    return String(args.a + args.b);
  },
});

// These must land on stderr, never on the JSON-RPC stdout stream.
console.log("debug line");
runtime.log("runtime line");

await srv.stdio();
