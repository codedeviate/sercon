// Demonstrates services.agentBrowser Phase 4 — debug/perf, the escape hatch,
// and the auth vault listing. Self-skips when the CLI is absent so `make demo`
// stays green. Network-free (data: URLs). Uses a short per-call timeout so a
// wedged daemon can't hang the demo.

if (!services.agentBrowser.available) {
  runtime.log("agent-browser CLI not found on PATH — skipping demo.");
} else {
  const html = "data:text/html," + encodeURIComponent("<title>adv demo</title><h1>hi</h1>");
  const b = services.agentBrowser.launch({ timeout: 8000 });
  try {
    await b.open(html);

    // Core Web Vitals.
    try {
      const v = await b.vitals();
      runtime.log("vitals ok:", v.success ?? true);
    } catch (e) { runtime.log("vitals skipped:", String((e as any).message ?? e)); }

    // Escape hatch: run an arbitrary agent-browser command (here: get title).
    try {
      const r = await b.cmd("get", "title");
      runtime.log("cmd get title:", JSON.stringify((r as any).data ?? r));
    } catch (e) { runtime.log("cmd skipped:", String((e as any).message ?? e)); }

    // batch: several commands in one round-trip (returns an array).
    try {
      const results = await b.batch(["get title", "get url"]);
      runtime.log("batch results:", Array.isArray(results) ? results.length : JSON.stringify(results));
    } catch (e) { runtime.log("batch skipped:", String((e as any).message ?? e)); }
  } finally {
    await b.close();
  }

  // Auth vault listing (namespace-level, no session needed).
  try {
    const profiles = await services.agentBrowser.auth.list();
    runtime.log("auth profiles:", JSON.stringify((profiles as any).data ?? profiles));
  } catch (e) { runtime.log("auth.list skipped:", String((e as any).message ?? e)); }

  runtime.log("done.");
}
