// Sercon.argv — the per-script argument vector.
// Layout mirrors Node/Bun: [programName, scriptPath, ...userArgs].
// Real arguments (those after `--` on the command line) start at index 2:
//   sercon examples/scripts/argv.ts -- --port 8080 hello
api.runtime.log("argv:", JSON.stringify(Sercon.argv));
api.runtime.log("program:", Sercon.argv[0]);
api.runtime.log("script:", Sercon.argv[1]);

const userArgs = Sercon.argv.slice(2);
api.runtime.log("user args:", JSON.stringify(userArgs));

// argv always holds at least [programName, scriptPath], even with no `--`
// args — so a script can read it unconditionally.
api.runtime.assert.ok(Sercon.argv.length >= 2, "argv should hold program + script");
api.runtime.assert.ok(Sercon.argv[1].endsWith("argv.ts"), "argv[1] is the running script");
