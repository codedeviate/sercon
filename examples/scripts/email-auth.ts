// Demonstrates api.email.* — every probe individually and the aggregate.
// All bindings return `{ present: false }` when the relevant record is
// missing or DNS NXDOMAINs; they throw on operational errors.

const target = "google.com";

api.log("=== api.email.spf:", target, "===");
const spf = await api.email.spf(target);
if (spf.present) {
  api.log("record:    ", spf.record);
  api.log("all policy:", spf.allPolicy);
} else {
  api.log("no SPF record");
}

api.log("");
api.log("=== api.email.dmarc:", target, "===");
const dmarc = await api.email.dmarc(target);
if (dmarc.present) {
  api.log("record:", dmarc.record);
  api.log("policy:", dmarc.policy, "subdomain:", dmarc.subdomain || "(inherits)");
  api.log("rua:   ", dmarc.rua || "(none)");
} else {
  api.log("no DMARC record");
}

api.log("");
api.log("=== api.email.mtaSts:", target, "===");
const mta = await api.email.mtaSts(target);
if (mta.present) {
  api.log("txt id:", mta.txt?.id);
  if (mta.policy) {
    api.log("mode:  ", mta.policy.mode);
    api.log("mx:    ", (mta.policy.mx ?? []).join(", "));
    api.log("max_age:", mta.policy.maxAge);
  } else if (mta.policyError) {
    api.log("policy fetch failed:", mta.policyError);
  }
} else {
  api.log("no MTA-STS record");
}

api.log("");
api.log("=== api.email.tlsRpt:", target, "===");
const tls = await api.email.tlsRpt(target);
if (tls.present) {
  api.log("record:", tls.record);
  api.log("rua:   ", tls.rua);
} else {
  api.log("no TLS-RPT record");
}

api.log("");
api.log("=== api.email.bimi:", target, "===");
const bimi = await api.email.bimi(target);
if (bimi.present) {
  api.log("record:  ", bimi.record);
  api.log("logo:    ", bimi.l);
  api.log("vmc:     ", bimi.a || "(none)");
} else {
  api.log("no BIMI record at", bimi.selector + "._bimi." + target);
}

api.log("");
api.log("=== api.email.all:", target, "===");
const all = await api.email.all(target);
for (const k of ["spf", "dmarc", "mtaSts", "tlsRpt", "bimi"]) {
  const probe = all[k];
  api.log(`  ${k}: ${probe.error ? "ERROR " + probe.error : probe.present ? "present" : "absent"}`);
}
