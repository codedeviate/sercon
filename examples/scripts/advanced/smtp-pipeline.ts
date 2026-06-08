// Demonstrates a full SMTP loopback round-trip: server.smtp.listen receives
// and parses a message; net.email.send delivers it locally; the captured
// parsed fields are asserted. Includes a multipart message with a plain-text
// body and an inline attachment so msg.attachments is exercised.

const port = 38201;

// Captured state from onData.
let captured: {
  subject: string;
  bodyText: string;
  from: string;
  to: string[];
  attachmentName: string;
} | null = null;

const srv = await server.smtp.listen({
  port,
  hostname: "pipe.local",
  handlers: {
    onMail: (_env: any) => true,
    onRcpt: (_env: any) => true,
    onData: (env: any, msg: any) => {
      const att = (msg.attachments ?? [])[0];
      captured = {
        subject: msg.subject,
        bodyText: msg.body?.text ?? "",
        from: env.from,
        to: env.to ?? [],
        attachmentName: att?.filename ?? "",
      };
      return true;
    },
  },
});

runtime.log("smtp server listening on", srv.address);

// Send a multipart message with an attached text file.
const attachmentContent = "attachment payload data";
const r = await net.email.send({
  from: "sender@pipe.local",
  to: "recipient@pipe.local",
  subject: "pipeline test subject",
  body: "hello from smtp-pipeline",
  attachments: [
    {
      filename: "notes.txt",
      content: attachmentContent,
      contentType: "text/plain",
    },
  ],
  server: { host: "127.0.0.1", port, tls: "none" },
});

runtime.log("send result: accepted:", r.accepted.length, "rejected:", r.rejected.length);

runtime.assert.equal(r.accepted.length, 1, "one accepted recipient");
runtime.assert.equal(r.rejected.length, 0, "no rejected recipients");
runtime.assert.ok(captured !== null, "message captured by server");

runtime.log("captured subject:", captured!.subject);
runtime.log("captured body excerpt:", captured!.bodyText.slice(0, 40));
runtime.log("captured from:", captured!.from);
runtime.log("captured attachment:", captured!.attachmentName);

runtime.assert.equal(captured!.subject, "pipeline test subject", "subject matches");
runtime.assert.ok(
  captured!.bodyText.includes("hello from smtp-pipeline"),
  "body contains expected text"
);
runtime.assert.equal(captured!.from, "sender@pipe.local", "from address matches");
runtime.assert.equal(captured!.attachmentName, "notes.txt", "attachment filename matches");

await srv.close();
runtime.log("PASS");
