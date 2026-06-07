// Demonstrates services.agentBrowser Phase 3 — cookies, web storage, tabs,
// network monitoring, and diffing. Self-skips when the CLI is absent so
// `make demo` stays green. Network-free (data: URLs).
//
// Note: cookies.set and storage.* require a real HTTP origin; on a data: URL
// the CLI exits non-zero (unavailable origin) — those paths are wrapped in
// try-catch so the demo stays informative and exits 0.

if (!services.agentBrowser.available) {
  runtime.log("agent-browser CLI not found on PATH — skipping demo.");
} else {
  const html = "data:text/html," + encodeURIComponent(
    "<title>state demo</title><h1 id=hi>Hello</h1>"
  );
  const b = services.agentBrowser.launch();
  try {
    await b.open(html);

    // Web storage — localStorage is unavailable on data: origins; the CLI
    // exits non-zero, so we catch and report without aborting.
    try {
      await b.storage.local.set("theme", "dark");
      const theme = await b.storage.local.get("theme");
      runtime.log("storage.local theme:", JSON.stringify((theme as any).data ?? theme));
    } catch (e: any) {
      runtime.log("storage.local (data: origin — unavailable):", String(e.message ?? e));
    }

    // Cookies — get() works on data: URLs (empty jar); set() needs an HTTP
    // origin so it is wrapped for the smoke-test.
    const cookies = await b.cookies.get();
    runtime.log("cookies count:", ((cookies as any).data?.cookies ?? []).length);
    try {
      await b.cookies.set("sid", "abc123", { sameSite: "Lax" });
      runtime.log("cookies.set: ok");
    } catch (e: any) {
      runtime.log("cookies.set (data: origin — unavailable):", String(e.message ?? e));
    }

    // Tabs — works on all origins.
    await b.tabs.new(html, { label: "second" });
    const tabs = await b.tabs.list();
    runtime.log("tabs:", JSON.stringify((tabs as any).data ?? tabs));
    await b.tabs.close("second");

    // Network request log (empty for a data: URL, but exercises the binding).
    const reqs = await b.network.requests({ clear: true });
    runtime.log("network.requests ok:", (reqs as any).success ?? true);

    // Snapshot diff against the current state (no changes → empty diff).
    const d = await b.diff.snapshot();
    runtime.log("diff.snapshot ok:", (d as any).success ?? true);
  } finally {
    await b.close();
    runtime.log("done.");
  }
}
