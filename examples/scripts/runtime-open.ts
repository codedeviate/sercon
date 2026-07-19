// runtime.open — hand a URL (or file path) to the OS default handler, i.e. the
// user's normal GUI browser. Feature-detected via runtime.openAvailable.
//
// Demo-safe: it only ACTUALLY opens when SERCON_OPEN_DEMO=1, so `make demo`
// never spawns a browser. Run it yourself with that env set to see it open.
const url = "https://example.com";
runtime.assert.equal(typeof runtime.openAvailable, "boolean", "openAvailable is a boolean");
runtime.log("openAvailable:", runtime.openAvailable);

if (runtime.env.get("SERCON_OPEN_DEMO") === "1" && runtime.openAvailable) {
  await runtime.open(url);
  runtime.log("opened:", url);
} else {
  runtime.log(`would open: ${url}  (set SERCON_OPEN_DEMO=1 to actually open)`);
}
