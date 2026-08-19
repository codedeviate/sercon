// runtime.stdout / runtime.stderr / runtime.stdin — script-controlled stdio.
// Redirect, silence, tee, capture, and feed a fake stdin.

// 1. capture() buffers a stream and hands you the text.
const captured = await runtime.stdout.capture(() => {
  console.log("captured line 1");
  console.log("captured line 2");
});
runtime.assert.equal(captured, "captured line 1\ncaptured line 2\n", "capture");

// 2. silence() swallows output; the restore fn brings it back.
const unsilence = runtime.stdout.silence();
console.log("you will never see this");
unsilence();

// 3. scoped() restores even when the body throws.
let threw = false;
try {
  await runtime.stdout.scoped("null", () => {
    throw new Error("boom");
  });
} catch {
  threw = true;
}
runtime.assert.ok(threw, "scoped must propagate the throw");

// 4. A file destination, with tee so it also reaches the terminal.
const logPath = "/tmp/sercon-stdio-demo.log";
const stopFile = runtime.stdout.toFile(logPath, { tee: true });
console.log("this line is on screen AND in the file");
stopFile();
runtime.assert.ok((await fs.readText(logPath)).includes("in the file"), "tee wrote the file");
await fs.remove(logPath);

// 5. A line callback: re-route each line into your own handler.
const collected: string[] = [];
const stopCB = runtime.stdout.to((line) => {
  collected.push(line);
});
console.log("routed-a");
console.log("routed-b");
await runtime.time.sleep(10); // delivery is scheduled on the loop
stopCB();
runtime.assert.equal(collected.join(","), "routed-a,routed-b", "line callback");

// 6. Fold stderr into stdout so one pipe captures everything.
const unfold = runtime.stderr.to("stdout");
console.error("this warning is on stdout now");
unfold();

// 7. Feed stdin from a string — the same script is now testable without a pipe.
runtime.stdin.fromString("alpha\nbeta\ngamma\n");
const lines: string[] = [];
for await (const line of runtime.stdin.lines()) {
  lines.push(line.toUpperCase());
}
runtime.assert.equal(lines.join(" "), "ALPHA BETA GAMMA", "stdin.lines");
runtime.stdin.reset();

runtime.log("stdio-redirect: all checks passed");
