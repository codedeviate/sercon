// runtime.argv — the per-script argument vector.
// Layout mirrors Node/Bun: [programName, scriptPath, ...userArgs].
// Real arguments (those after `--` on the command line) start at index 2:
//   sercon examples/scripts/argv.ts -- --port 8080 hello
runtime.log("argv:", JSON.stringify(runtime.argv));
runtime.log("program:", runtime.argv[0]);
runtime.log("script:", runtime.argv[1]);

const userArgs = runtime.argv.slice(2);
runtime.log("user args:", JSON.stringify(userArgs));

// argv always holds at least [programName, scriptPath], even with no `--`
// args — so a script can read it unconditionally.
runtime.assert.ok(runtime.argv.length >= 2, "argv should hold program + script");
runtime.assert.ok(runtime.argv[1].endsWith("argv.ts"), "argv[1] is the running script");
