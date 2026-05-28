api.runtime.log("hello from ts");
api.runtime.assert.equal(1 + 1, 2);
api.runtime.assert.ok(api.runtime.time.nowMs() > 0);
