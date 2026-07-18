// fs.find — fast file search (fd-like), fully offline against an in-script tree.
const dir = `${runtime.env.get("TMPDIR") ?? "/tmp"}/sercon-find-${runtime.time.nowMs()}`;
await fs.mkdir(`${dir}/sub`);
await fs.writeText(`${dir}/a.go`, "package a\n");
await fs.writeText(`${dir}/sub/b.go`, "package b\n");
await fs.writeText(`${dir}/notes.md`, "# notes\n");

const go = await fs.find({ root: dir, glob: "**/*.go" });
runtime.assert.equal(go.length, 2, "two .go files");

const md = await fs.find({ root: dir, extension: "md", stat: true });
runtime.assert.equal(md.length, 1, "one .md");
runtime.assert.equal(md[0].type, "file", "stat type");

await fs.remove(dir);
runtime.log("fs.find OK");
