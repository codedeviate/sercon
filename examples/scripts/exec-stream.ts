// services.exec.stream — run a subprocess and receive its output line by line
// as it arrives, instead of buffering it all (services.exec.shell). Resolves
// { exitCode, success, durationMs } on exit.

const lines: string[] = [];
const result = await services.exec.stream(
  "echo one; echo two; echo err 1>&2",
  (line: string, stream: string) => {
    lines.push(`${stream}:${line}`);
    runtime.log(stream, line);
  },
);

runtime.log("exit", result.exitCode, "success", result.success);

// Self-check: we should have seen both stdout lines and the stderr line.
const sawStdout = lines.includes("stdout:one") && lines.includes("stdout:two");
const sawStderr = lines.includes("stderr:err");
if (!result.success || !sawStdout || !sawStderr) {
  throw new Error("exec-stream self-test failed: " + JSON.stringify(lines));
}
runtime.log("exec-stream self-test PASS");
