// stuffr — pack/list/test/unpack round trip, gzip via stdin, formats.
// Offline and self-contained: stuffr is embedded WebAssembly (no external
// tools). Only content-level behaviour is asserted; the interim build does
// not preserve modes/symlinks/owners on extract, so those are not checked.
const base = runtime.env.get("TMPDIR") ?? "/tmp";
const dir = `${base}/sercon-stuffr-${Date.now()}`;
const text = "hello from stuffr\n";

await fs.remove(dir);
await fs.mkdir(dir);
try {
  await fs.writeText(`${dir}/a.txt`, text);

  // pack -> list -> test -> unpack, content round trip.
  await stuffr.pack(["a.txt"], { cwd: dir, output: "a.tar.gz" });
  const entries: any[] = await stuffr.list(`${dir}/a.tar.gz`);
  runtime.assert.equal(entries.length, 1, "one entry");
  runtime.assert.equal(entries[0].name, "a.txt", "entry name");
  await stuffr.test(`${dir}/a.tar.gz`);
  const un = await stuffr.unpack(`${dir}/a.tar.gz`, { directory: `${dir}/out` });
  runtime.assert.ok(Array.isArray(un.warnings), "unpack resolves { warnings }");
  runtime.assert.equal(await fs.readText(`${dir}/out/a.txt`), text, "content round trip");

  // gzip: pack a single file, then decompress it through stdin.
  await stuffr.pack(["a.txt"], { cwd: dir, output: "a.txt.gz", format: "gzip" });
  const gz = await fs.readBytes(`${dir}/a.txt.gz`);
  const r = await stuffr.run(["cat", "-"], { stdin: gz });
  runtime.assert.equal(r.code, 0, "cat exit code");
  runtime.assert.equal(r.text(), text, "gzip stdin round trip");

  // formats: the embedded build reports what it supports.
  const f: any = await stuffr.formats();
  const names: string[] = f.formats.map((x: any) => x.format);
  runtime.assert.ok(names.includes("gzip") && names.includes("zip"), "formats lists gzip and zip");

  runtime.log("stuffr OK:", stuffr.version, "-", names.length, "formats");
} finally {
  await fs.remove(dir);
}
