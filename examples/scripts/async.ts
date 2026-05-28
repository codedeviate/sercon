import { check } from "./helpers/assert";

const r = await api.net.http.get("https://example.com");
check(r.status === 200, `expected 200, got ${r.status}`);
api.runtime.log("body length:", r.body.length);
await api.runtime.time.sleep(50);
api.runtime.log("done");
