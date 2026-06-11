// runtime.secrets — read/write credentials in the OS keystore (macOS Keychain,
// Linux Secret Service, Windows Credential Manager). Pure-Go, no cgo.
//
// Self-skips when no keystore backend is reachable (e.g. a headless CI box),
// so it is safe to run anywhere. Operations are confined to a prefix namespace
// (keystore service = prefix + name; default "sercon/", override with
// --secrets-prefix / SERCON_SECRETS_PREFIX).

if (!runtime.secrets.available) {
  runtime.log("no OS keystore backend reachable — skipping runtime.secrets demo.");
} else {
  const name = "sercon-selftest";
  const account = "demo@example.com";

  // store, read back, delete — a full round-trip in the sercon/ namespace.
  await runtime.secrets.set(name, account, "s3cr3t-value");
  const got = await runtime.secrets.get(name, account);
  runtime.assert.equal(got, "s3cr3t-value", "round-tripped secret should match");
  runtime.log("stored + read back:", got);

  const removed = await runtime.secrets.delete(name, account);
  runtime.assert.ok(removed, "delete should report the item was removed");

  const after = await runtime.secrets.get(name, account);
  runtime.assert.equal(after, null, "secret should be gone after delete");
  runtime.log("deleted; get now returns:", after);

  runtime.log("runtime.secrets round-trip ok");
}
