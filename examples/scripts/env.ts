// Demonstrates programmatic .env: codec.dotenv.parse/stringify (pure) and
// runtime.env.load (applies to the environment). Offline + self-contained.
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";
const path = `${tmp}/sercon-demo.env`;

// Pure parse.
const cfg = codec.dotenv.parse('GREETING="hello world"\n# comment\nexport COUNT=3\n');
runtime.assert.equal(cfg.GREETING, "hello world", "parse strips quotes");
runtime.assert.equal(cfg.COUNT, "3", "parse handles export");

// Round-trip stringify → parse.
const text = codec.dotenv.stringify({ A: "1", B: "two words", FLAG: true });
runtime.assert.equal(codec.dotenv.parse(text).B, "two words", "stringify round-trips");

// Load a file into the environment.
await fs.writeText(path, 'DEMO_KEY="from file"\n');
const loaded = await runtime.env.load(path);
runtime.assert.equal(loaded.DEMO_KEY, "from file", "load returns parsed pairs");
runtime.assert.equal(runtime.env.get("DEMO_KEY"), "from file", "load applied to env");

runtime.log("env OK:", JSON.stringify(loaded), "| stringify:", JSON.stringify(text));
