// Demonstrates fs.archive.* — create + extract over zip / tar / tar.gz.
// Uses a few temporary files under the host's temp dir so the script
// cleans up after itself.

const tmp = runtime.env.get("TMPDIR") ?? "/tmp";
const work = fs.path.dirname(tmp + "/sercon-archive-demo");

// Build a tiny fixture tree on disk via shell? We don't have a shell
// binding yet. Instead, exercise the API against a couple of paths the
// caller's environment is likely to have (README, CHANGELOG). The point
// is to show round-trip mechanics, not produce a real backup.

const sources = ["README.md", "CHANGELOG.md", "LICENSE"];

for (const ext of [".zip", ".tar", ".tar.gz"]) {
  const dest = tmp + "/sercon-demo" + ext;
  const out = await fs.archive.create(dest, sources);
  runtime.log(`  ${ext.padEnd(7)} ${out.bytes.toString().padStart(6)} bytes  entries=${out.entries.length}  -> ${dest}`);

  // Extract into a sibling directory and list what came back.
  const dir = tmp + "/sercon-demo-extracted-" + ext.replace(/[^a-z]/g, "");
  const got = await fs.archive.extract(dest, dir, { overwrite: true });
  runtime.log(`         extracted ${got.entries.length} entries  -> ${dir}`);
}

runtime.log("");
runtime.log("Tip: fs.archive.create also takes [{path, name}] objects to override the in-archive name.");
