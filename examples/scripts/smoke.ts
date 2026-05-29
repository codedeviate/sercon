runtime.log("hello from ts");
runtime.assert.equal(1 + 1, 2);
runtime.assert.ok(runtime.time.nowMs() > 0);
