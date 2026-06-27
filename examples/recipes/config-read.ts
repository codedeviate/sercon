// Recipe: load app config from TOML and JSON and use the values.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const toml = codec.toml.parse(await fs.readText(data("config.toml")));
const json = JSON.parse(await fs.readText(data("config.json")));
runtime.assert.equal(toml.server.port, 8080, "toml port");
runtime.assert.equal(json.server.port, 8080, "json port");
runtime.assert.equal(toml.title, json.title, "toml and json agree on title");
runtime.log("config-read:", toml.title, "— listening on", `${toml.server.host}:${toml.server.port}`, "log level", json.logging.level);
