import { check } from "./helpers/assert";
import { netSkip } from "./helpers/netskip";

// Hits example.com, so it's part of the network demo set (not CI). Self-skips
// (exit 0) when the network is unreachable; a real failure is re-thrown.
try {
  const r = await net.http.get("https://example.com");
  check(r.status === 200, `expected 200, got ${r.status}`);
  runtime.log("body length:", r.body.length);
  await runtime.time.sleep(50);
  runtime.log("done");
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("example.com unreachable — skipping async demo. (" + String(e).slice(0, 120) + ")");
}
