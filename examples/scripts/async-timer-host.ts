// Regression guard: a host async binding awaited immediately after an awaited
// setTimeout-backed promise must still run its continuation. The event loop
// used to drain the moment the timer dropped its job count to zero, silently
// skipping the host call's tail (the script "succeeded" with exit 0 but the
// work never ran). See CLAUDE.md § "Keeping the event loop alive across async
// work" and pkg/scriptengine bumpLoopSync.
function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

await sleep(50);
const r = await services.exec.shell("echo hi-from-shell");
runtime.assert.equal(r.stdout.trim(), "hi-from-shell", "host call after timer await ran its continuation");

// Also cover the runtime.time.sleep helper feeding a host call.
await runtime.time.sleep(20);
const r2 = await services.exec.shell("echo second");
runtime.assert.equal(r2.stdout.trim(), "second", "second timer->host sequence completed");

runtime.log("async timer->host OK");
