// Demonstrates api.db.ldap.* — anonymous LDAP query over go-ldap/v3.
// Gracefully degrades when no LDAP server is reachable.

const url = api.runtime.env.get("LDAP_URL") ?? "ldap://ldap.forumsys.com:389";
api.runtime.log("connecting to", url, "...");

try {
  const l = await api.db.ldap.open(url);

  // The Root DSE advertises the server's capabilities + naming contexts.
  const dse = await api.db.ldap.rootDSE ? await l.rootDSE() : {};
  const contexts = dse.namingContexts ?? dse.namingcontexts ?? [];
  api.runtime.log("naming contexts:", Array.isArray(contexts) ? contexts.join(", ") : "(none advertised)");

  // A subtree search (forumsys public test directory).
  const entries = await l.search("dc=example,dc=com", "(objectClass=*)", ["dn"]);
  api.runtime.log("entries under dc=example,dc=com:", entries.length);
  for (const e of entries.slice(0, 3)) api.runtime.log("  -", e.dn);

  await l.close();
} catch (e) {
  api.runtime.log("no LDAP reachable — skipping:", String(e).slice(0, 60));
  api.runtime.log("(set LDAP_URL to a reachable server to see it live)");
}
