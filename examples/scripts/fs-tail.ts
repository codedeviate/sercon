// fs.tail — follow a growing file (like `tail -f`). Self-terminating demo:
// append a few lines via a background shell, tail from the end, stop after them.
const path = `${runtime.env.get("TMPDIR") ?? "/tmp"}/sercon-tail-${runtime.time.nowMs()}.log`;
await fs.writeText(path, ""); // create empty

// Append 3 lines in the background (>> genuinely appends; no truncation race).
(async () => {
  for (const msg of ["line 1", "line 2", "line 3"]) {
    await runtime.time.sleep(40);
    await services.exec.shell(["/bin/sh", "-c", `printf '%s\\n' ${JSON.stringify(msg)} >> ${JSON.stringify(path)}`]);
  }
})();

const got: string[] = [];
for await (const line of fs.tail(path)) {   // from:"end" — only the appended lines
  got.push(line);
  if (got.length === 3) break;              // stop the follow
}
runtime.assert.equal(got.join(","), "line 1,line 2,line 3", "tail followed the appends");
await fs.remove(path);
runtime.log("fs.tail OK");
