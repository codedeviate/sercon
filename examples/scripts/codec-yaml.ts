// codec.yaml — parse a YAML document and round-trip a value back to YAML.
const cfg = codec.yaml.parse(`
title: sercon demo
server:
  port: 8080
  hosts: [a.example, b.example]
`);
runtime.assert.equal(cfg.server.port, 8080, "parsed int");
runtime.assert.equal(cfg.server.hosts.length, 2, "parsed sequence");

// Top-level sequence parses to an array.
const list = codec.yaml.parse("- one\n- two\n");
runtime.assert.equal(list[1], "two", "top-level sequence");

const text = codec.yaml.stringify({ name: "demo", flags: { debug: true } });
const back = codec.yaml.parse(text);
runtime.assert.equal(back.flags.debug, true, "round-trip bool");
runtime.log("codec.yaml OK:", cfg.title, "/", back.name);
