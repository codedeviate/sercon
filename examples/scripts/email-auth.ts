// Demonstrates api.email.* — SPF + DMARC lookups for a domain. Both bindings
// return `{ present: false }` when the relevant record is missing or DNS
// NXDOMAINs; they throw on operational errors (resolver unreachable, etc.).

const target = "google.com";

api.log("=== api.email.spf:", target, "===");
const spf = await api.email.spf(target);
if (spf.present) {
  api.log("record:    ", spf.record);
  api.log("all policy:", spf.allPolicy);
  api.log("mechanisms:", spf.mechanisms.length, "tokens");
} else {
  api.log("no SPF record published");
}

api.log("");
api.log("=== api.email.dmarc:", target, "===");
const dmarc = await api.email.dmarc(target);
if (dmarc.present) {
  api.log("record:    ", dmarc.record);
  api.log("policy:    ", dmarc.policy);
  api.log("subdomain: ", dmarc.subdomain || "(inherits)");
  api.log("percent:   ", dmarc.percent || "(default 100)");
  api.log("rua:       ", dmarc.rua || "(none)");
} else {
  api.log("no DMARC record published at _dmarc." + target);
}
