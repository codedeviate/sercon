import { check } from "./helpers/assert";

const r = await api.http.get("https://example.com");
check(r.status === 200, `expected 200, got ${r.status}`);
api.log("body length:", r.body.length);
await api.time.sleep(50);
api.log("done");
