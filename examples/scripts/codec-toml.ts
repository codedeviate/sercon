// codec.toml — parse a TOML document and round-trip a value back to TOML.
const cfg = codec.toml.parse(`
title = "sercon demo"
[server]
port = 8080
hosts = ["a.example", "b.example"]
`);
runtime.assert.equal(cfg.server.port, 8080, "parsed int");
runtime.assert.equal(cfg.server.hosts.length, 2, "parsed array");

const text = codec.toml.stringify({ name: "demo", flags: { debug: true } });
const back = codec.toml.parse(text);
runtime.assert.equal(back.flags.debug, true, "round-trip bool");
runtime.log("codec.toml OK:", cfg.title, "/", back.name);
