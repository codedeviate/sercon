// fs.tail — follow a growing file (like `tail -f`). Self-terminating demo:
// append a few lines in the background, tail from the end, stop after them.
const path = `${runtime.env.get("TMPDIR") ?? "/tmp"}/sercon-tail-${runtime.time.nowMs()}.log`;
await fs.writeText(path, ""); // create empty

// appendLine appends one line without a shell: `tee -a` gets the path as a
// literal argv element (no shell interpolation → no injection) and the content
// via stdin. This genuinely appends (no truncate-then-rewrite race with tail).
const appendLine = (file: string, text: string) =>
  services.exec.shell(["tee", "-a", file], { stdin: text + "\n" });

(async () => {
  for (const msg of ["line 1", "line 2", "line 3"]) {
    await runtime.time.sleep(40);
    await appendLine(path, msg);
  }
})();

const got: string[] = [];
for await (const line of fs.tail(path)) {   // from:"end" — only the appended lines
  got.push(line);
  if (got.length === 3) break;              // stop the follow
}
runtime.assert.equal(got.join(","), "line 1,line 2,line 3", "tail followed the appends");
await fs.remove(path);

// fs.grepStream — follow + match (tail | grep). Catch the first ERROR line.
const errPath = `${runtime.env.get("TMPDIR") ?? "/tmp"}/sercon-gs-${runtime.time.nowMs()}.log`;
await fs.writeText(errPath, "");
(async () => {
  for (const msg of ["info: up", "ERROR: disk full", "info: retry"]) {
    await runtime.time.sleep(40);
    await appendLine(errPath, msg);
  }
})();
for await (const m of fs.grepStream(errPath, { pattern: "ERROR", fixed: true })) {
  runtime.assert.ok(m.text.includes("disk full"), "matched the ERROR line");
  break;
}
await fs.remove(errPath);

runtime.log("fs.tail OK");
