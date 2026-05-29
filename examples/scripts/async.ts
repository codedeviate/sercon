import { check } from "./helpers/assert";

const r = await net.http.get("https://example.com");
check(r.status === 200, `expected 200, got ${r.status}`);
runtime.log("body length:", r.body.length);
await runtime.time.sleep(50);
runtime.log("done");
