// Demonstrates services.agentBrowser.* — the core automation loop.
// Self-skips when the agent-browser CLI is not installed, so `make demo`
// stays green on machines without it. Uses a data: URL so it never hits
// the network.
//
// agent-browser returns { success, data, error } envelopes; drill into .data
// for the actual values (e.g. data.title, data.value, data.visible).

if (!services.agentBrowser.available) {
  runtime.log("agent-browser CLI not found on PATH — skipping demo.");
} else {
  const html = "data:text/html," + encodeURIComponent(
    "<title>sercon demo</title><h1 id=hi>Hello</h1><input id=box>"
  );
  const b = services.agentBrowser.launch({ headed: false });
  try {
    await b.open(html);

    // get("title") → { success, data: { title }, error }
    const titleRes = await b.get("title");
    runtime.log("title:", (titleRes as any).data?.title);

    // fill then get value
    await b.fill("#box", "typed by sercon");
    const valRes = await b.get("value", "#box");
    runtime.log("value:", (valRes as any).data?.value);

    // isVisible → { success, data: { visible, ... }, error }
    const visRes = await b.isVisible("#hi");
    runtime.log("h1 visible:", (visRes as any).data?.visible);

    // snapshot → { success, data: {...tree...}, error }
    const snap = await b.snapshot({ interactive: true });
    runtime.log("snapshot keys:", Object.keys(snap).join(", "));
  } finally {
    await b.close();
    runtime.log("session closed.");
  }
}
