// fs.grep family — content search (rg-like), offline against an in-script tree.
const dir = `${runtime.env.get("TMPDIR") ?? "/tmp"}/sercon-grep-${runtime.time.nowMs()}`;
await fs.mkdir(dir);
await fs.writeText(`${dir}/a.go`, "package a\n// TODO: fix\n");
await fs.writeText(`${dir}/b.go`, "package b\n");

const hits = await fs.grep({ root: dir, pattern: "TODO", fixed: true });
runtime.assert.equal(hits.length, 1, "one TODO");
runtime.assert.equal(hits[0].line, 2, "line 2");

const files = await fs.grepFiles({ root: dir, pattern: "package" });
runtime.assert.equal(files.length, 2, "two files");

const counts = await fs.grepCount({ root: dir, pattern: "package" });
runtime.assert.equal(counts.length, 2, "two counts");

await fs.remove(dir);
runtime.log("fs.grep OK");
