// Demonstrates server.smtp.listen + net.email.send — self-contained
// round-trip: bind a port, send a message to ourselves, assert receipt.

const port = 38095;

let captured: { subject: string; body: string; from: string } | null = null;

const srv = await server.smtp.listen({
  port,
  hostname: "test.local",
  handlers: {
    onMail: () => true,
    onRcpt: () => true,
    onData: (env: any, msg: any) => {
      captured = { subject: msg.subject, body: msg.body.text, from: env.from };
      return true;
    },
  },
});

runtime.log("listening on", srv.address);

const r = await net.email.send({
  to: "alice@test.local",
  from: "bob@test.local",
  subject: "round-trip demo",
  body: "hello from the SMTP demo",
  server: { host: "127.0.0.1", port, tls: "none" },
});

runtime.assert.equal(r.accepted.length, 1, "accepted count");
runtime.assert.equal(r.rejected.length, 0, "rejections");
runtime.assert.ok(captured !== null, "captured");
runtime.assert.equal(captured!.subject, "round-trip demo", "subject");
runtime.assert.ok(captured!.body.includes("hello from"), "body");

await srv.close();
runtime.log("ok");
