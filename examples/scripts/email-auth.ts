// Demonstrates net.email.* — every probe individually and the aggregate.
// All bindings return `{ present: false }` when the relevant record is
// missing or DNS NXDOMAINs; they throw on operational errors (e.g. a DNS
// timeout). It does live DNS lookups, so it's in the network demo set (not
// CI) and self-skips (exit 0) when DNS is unreachable; genuine errors are
// re-thrown.

import { netSkip } from "./helpers/netskip";

const target = "google.com";

try {
  runtime.log("=== net.email.spf:", target, "===");
  const spf = await net.email.spf(target);
  if (spf.present) {
    runtime.log("record:    ", spf.record);
    runtime.log("all policy:", spf.allPolicy);
  } else {
    runtime.log("no SPF record");
  }

  runtime.log("");
  runtime.log("=== net.email.dmarc:", target, "===");
  const dmarc = await net.email.dmarc(target);
  if (dmarc.present) {
    runtime.log("record:", dmarc.record);
    runtime.log("policy:", dmarc.policy, "subdomain:", dmarc.subdomain || "(inherits)");
    runtime.log("rua:   ", dmarc.rua || "(none)");
  } else {
    runtime.log("no DMARC record");
  }

  runtime.log("");
  runtime.log("=== net.email.mtaSts:", target, "===");
  const mta = await net.email.mtaSts(target);
  if (mta.present) {
    runtime.log("txt id:", mta.txt?.id);
    if (mta.policy) {
      runtime.log("mode:  ", mta.policy.mode);
      runtime.log("mx:    ", (mta.policy.mx ?? []).join(", "));
      runtime.log("max_age:", mta.policy.maxAge);
    } else if (mta.policyError) {
      runtime.log("policy fetch failed:", mta.policyError);
    }
  } else {
    runtime.log("no MTA-STS record");
  }

  runtime.log("");
  runtime.log("=== net.email.tlsRpt:", target, "===");
  const tls = await net.email.tlsRpt(target);
  if (tls.present) {
    runtime.log("record:", tls.record);
    runtime.log("rua:   ", tls.rua);
  } else {
    runtime.log("no TLS-RPT record");
  }

  runtime.log("");
  runtime.log("=== net.email.bimi:", target, "===");
  const bimi = await net.email.bimi(target);
  if (bimi.present) {
    runtime.log("record:  ", bimi.record);
    runtime.log("logo:    ", bimi.l);
    runtime.log("vmc:     ", bimi.a || "(none)");
  } else {
    runtime.log("no BIMI record at", bimi.selector + "._bimi." + target);
  }

  runtime.log("");
  runtime.log("=== net.email.all:", target, "===");
  const all = await net.email.all(target);
  for (const k of ["spf", "dmarc", "mtaSts", "tlsRpt", "bimi"]) {
    const probe = all[k];
    runtime.log(`  ${k}: ${probe.error ? "ERROR " + probe.error : probe.present ? "present" : "absent"}`);
  }
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("DNS unreachable — skipping email-auth demo. (" + String(e).slice(0, 120) + ")");
}
