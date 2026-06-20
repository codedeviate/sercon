// Demonstrates runtime.setDeadline / clearDeadline / getDeadline — control the
// script's own wall-clock kill timeout at runtime. (Distinct from the JS global
// setTimeout, which schedules a callback.) Offline; in make demo.

// Under `sercon` the default deadline is 10s; getDeadline reports ms remaining.
const start = runtime.getDeadline();
runtime.log("initial deadline (ms remaining):", start);

// Move the deadline to 5s from now.
runtime.setDeadline(5000);
const after = runtime.getDeadline();
runtime.assert.ok(after !== null && after > 0 && after <= 5000, "deadline reset to <=5s");
runtime.log("after setDeadline(5000):", after);

// A short sleep, well within the deadline.
await runtime.time.sleep(50);

// Remove the deadline entirely (like -timeout 0).
runtime.clearDeadline();
runtime.assert.equal(runtime.getDeadline(), null, "deadline cleared → null");
runtime.log("after clearDeadline():", runtime.getDeadline());

runtime.log("deadline demo OK");
