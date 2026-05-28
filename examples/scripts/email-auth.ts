// Demonstrates api.net.email.* — every probe individually and the aggregate.
// All bindings return `{ present: false }` when the relevant record is
// missing or DNS NXDOMAINs; they throw on operational errors.

const target = "google.com";

api.runtime.log("=== api.net.email.spf:", target, "===");
const spf = await api.net.email.spf(target);
if (spf.present) {
  api.runtime.log("record:    ", spf.record);
  api.runtime.log("all policy:", spf.allPolicy);
} else {
  api.runtime.log("no SPF record");
}

api.runtime.log("");
api.runtime.log("=== api.net.email.dmarc:", target, "===");
const dmarc = await api.net.email.dmarc(target);
if (dmarc.present) {
  api.runtime.log("record:", dmarc.record);
  api.runtime.log("policy:", dmarc.policy, "subdomain:", dmarc.subdomain || "(inherits)");
  api.runtime.log("rua:   ", dmarc.rua || "(none)");
} else {
  api.runtime.log("no DMARC record");
}

api.runtime.log("");
api.runtime.log("=== api.net.email.mtaSts:", target, "===");
const mta = await api.net.email.mtaSts(target);
if (mta.present) {
  api.runtime.log("txt id:", mta.txt?.id);
  if (mta.policy) {
    api.runtime.log("mode:  ", mta.policy.mode);
    api.runtime.log("mx:    ", (mta.policy.mx ?? []).join(", "));
    api.runtime.log("max_age:", mta.policy.maxAge);
  } else if (mta.policyError) {
    api.runtime.log("policy fetch failed:", mta.policyError);
  }
} else {
  api.runtime.log("no MTA-STS record");
}

api.runtime.log("");
api.runtime.log("=== api.net.email.tlsRpt:", target, "===");
const tls = await api.net.email.tlsRpt(target);
if (tls.present) {
  api.runtime.log("record:", tls.record);
  api.runtime.log("rua:   ", tls.rua);
} else {
  api.runtime.log("no TLS-RPT record");
}

api.runtime.log("");
api.runtime.log("=== api.net.email.bimi:", target, "===");
const bimi = await api.net.email.bimi(target);
if (bimi.present) {
  api.runtime.log("record:  ", bimi.record);
  api.runtime.log("logo:    ", bimi.l);
  api.runtime.log("vmc:     ", bimi.a || "(none)");
} else {
  api.runtime.log("no BIMI record at", bimi.selector + "._bimi." + target);
}

api.runtime.log("");
api.runtime.log("=== api.net.email.all:", target, "===");
const all = await api.net.email.all(target);
for (const k of ["spf", "dmarc", "mtaSts", "tlsRpt", "bimi"]) {
  const probe = all[k];
  api.runtime.log(`  ${k}: ${probe.error ? "ERROR " + probe.error : probe.present ? "present" : "absent"}`);
}
