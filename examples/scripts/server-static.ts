// Demonstrates server.http.static — serve a directory tree at a URL prefix.

const port = 38081;
const dir = await services.exec.shell("mktemp -d");
const root = dir.stdout.trim();

// Drop a known file.
await services.exec.shell(["sh", "-c", `printf 'static body' > ${root}/hello.txt`]);

const srv = await server.http.listen({
  port,
  routes: {
    "GET /assets/{rest...}": server.http.static({
      dir: root,
      stripPrefix: "/assets/",
    }),
    "GET /": (req: any, res: any) => res.text("home"),
  },
});

const r = await net.http.get(`http://127.0.0.1:${port}/assets/hello.txt`);
runtime.assert.equal(r.status, 200, "status");
runtime.assert.equal(r.body, "static body", "body");

await srv.close();
await services.exec.shell(["rm", "-rf", root]);
runtime.log("ok");
