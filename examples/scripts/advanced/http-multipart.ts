// Demonstrates net.http.request multipart/form-data uploads (and a binary
// body) fully offline: a local server.http route receives the upload, asserts
// the raw request carries the expected multipart markers, and acknowledges it;
// the client verifies the round-trip. No network, no credentials.

const port = 38096;
const srv = await server.http.listen({
  port,
  routes: {
    "POST /upload": (q: any, r: any) => {
      const ct = (q.headers["content-type"] ?? [""])[0];
      runtime.assert.ok(
        ct.startsWith("multipart/form-data; boundary="),
        "server saw a multipart content-type",
      );
      runtime.assert.ok(q.body.includes('name="title"'), "body carries the text field");
      runtime.assert.ok(q.body.includes('filename="note.txt"'), "body carries the file part");
      runtime.assert.ok(q.body.includes("hello from sercon"), "body carries the file content");
      return r.json({ received: true });
    },
    "POST /raw": (q: any, r: any) => {
      runtime.assert.equal(q.bodyBytes.length, 6, "raw binary body length");
      return r.json({ len: q.bodyBytes.length });
    },
  },
});

try {
  const up = await net.http.request("POST", `http://127.0.0.1:${port}/upload`, {
    multipart: [
      { name: "title", value: "My upload" },
      { name: "note", filename: "note.txt", content: "hello from sercon", type: "text/plain" },
    ],
  });
  runtime.assert.equal(up.status, 200, "multipart upload status");
  runtime.assert.ok(JSON.parse(up.body).received, "server acknowledged multipart");
  runtime.log("net.http multipart upload OK");

  const raw = await net.http.request("POST", `http://127.0.0.1:${port}/raw`, {
    body: new Uint8Array([0, 255, 10, 13, 127, 128]),
  });
  runtime.assert.equal(raw.status, 200, "binary body status");
  runtime.log("net.http binary body OK");
} finally {
  await srv.close();
}
