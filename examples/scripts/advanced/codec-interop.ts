// Advanced demo: round-trip a structured value across five encodings and
// assert each one reproduces the original.
//
// Codecs exercised:
//   codec.php.serialize / .unserialize
//   codec.perl.dumper / .parseDumper
//   codec.xml.encode / .decode
//   JSON.stringify / JSON.parse
//   codec.compression.compress / .decompress  (gzip on the JSON form)
//
// Quirks documented inline:
//   - PHP/Perl parsers return numbers as numbers and booleans as booleans,
//     so a plain JSON.stringify comparison works for the plain-object form.
//   - PHP serialize requires a "__class" key to encode as an object literal
//     rather than a plain array; the key is preserved in unserialize output.
//   - Perl booleans use the JSON::XS::Boolean convention on the wire but
//     parseDumper decodes them back to JS true/false.
//   - XML values are always strings after decode; we compare via a
//     normalised JSON projection with all values converted to strings.
//   - codec.compression functions return ArrayBuffer; convert to Uint8Array
//     for iteration, or pass directly to decompress.

// ── fixture value ─────────────────────────────────────────────────────────
// A plain object (no __class) so every codec round-trips it cleanly.
const original = {
  id:      42,
  name:    "Sercon interop",
  score:   9.75,
  active:  true,
  tags:    ["codec", "demo"],
  meta:    null as null,
};

// Helper: deep-equal via canonical JSON.
const sameJSON = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

// For XML, every field comes back as a string.
const asStrings = (o: typeof original) => ({
  id:     String(o.id),
  name:   o.name,
  score:  String(o.score),
  active: String(o.active),
  // tags array is preserved by the XML codec as-is (array of strings)
  tags:   o.tags,
  meta:   o.meta,   // null stays null (XML codec: absent element)
});

// ── track results ─────────────────────────────────────────────────────────
const results: Array<{ codec: string; ok: boolean; note: string }> = [];

// ── 1. PHP serialize / unserialize ────────────────────────────────────────
{
  const encoded  = codec.php.serialize(original);
  const decoded  = codec.php.unserialize(encoded);
  const ok       = sameJSON(decoded, original);
  results.push({ codec: "php.serialize", ok, note: ok ? "" : JSON.stringify(decoded) });
  runtime.assert.ok(ok, "PHP serialize round-trip failed");
}

// ── 2. Perl Data::Dumper / parseDumper ────────────────────────────────────
{
  const encoded  = codec.perl.dumper(original);
  const decoded  = codec.perl.parseDumper(encoded);
  const ok       = sameJSON(decoded, original);
  results.push({ codec: "perl.dumper", ok, note: ok ? "" : JSON.stringify(decoded) });
  runtime.assert.ok(ok, "Perl Data::Dumper round-trip failed");
}

// ── 3. codec.xml.encode / .decode ─────────────────────────────────────────
// XML cannot represent null children inline; omit the null key for encoding
// and compare without it. Everything else round-trips as strings.
{
  const forXml = {
    item: {
      "@id": String(original.id),
      name:  original.name,
      score: String(original.score),
      active: String(original.active),
      // tags: repeated sibling <tag> elements
      tag:  original.tags,
    },
  };

  const xml     = codec.xml.encode(forXml, { indent: "  ", declaration: true });
  const decoded = codec.xml.decode(xml) as typeof forXml;

  // Compare the structure round-trip.
  const ok = sameJSON(decoded, forXml);
  results.push({
    codec: "codec.xml",
    ok,
    note: ok ? "" : JSON.stringify(decoded),
  });
  runtime.assert.ok(ok, "XML encode/decode round-trip failed");
}

// ── 4. JSON.stringify / JSON.parse ─────────────────────────────────────────
{
  const encoded  = JSON.stringify(original);
  const decoded  = JSON.parse(encoded);
  const ok       = sameJSON(decoded, original);
  results.push({ codec: "JSON", ok, note: ok ? "" : JSON.stringify(decoded) });
  runtime.assert.ok(ok, "JSON round-trip failed");
}

// ── 5. gzip compression on the JSON form ──────────────────────────────────
{
  const json       = JSON.stringify(original);
  const compressed = await codec.compression.compress("gzip", json);
  const raw        = await codec.compression.decompress("gzip", compressed);

  // decompress returns ArrayBuffer; convert to string.
  const u8         = new Uint8Array(raw);
  const backStr    = Array.from(u8).map((b) => String.fromCharCode(b)).join("");
  const decoded    = JSON.parse(backStr);
  const ok         = sameJSON(decoded, original);

  const u8c        = new Uint8Array(compressed);
  const ratio      = ((u8c.length / json.length) * 100).toFixed(1);
  results.push({
    codec: "gzip(JSON)",
    ok,
    note: ok ? `${u8c.length}B → ${json.length}B (${ratio}%)` : JSON.stringify(decoded),
  });
  runtime.assert.ok(ok, "gzip compression round-trip failed");
}

// ── summary table ──────────────────────────────────────────────────────────
runtime.log("");
runtime.log("codec round-trip results:");
runtime.log("  codec           ok   note");
runtime.log("  " + "─".repeat(54));
for (const r of results) {
  const status = r.ok ? "PASS" : "FAIL";
  const note   = r.note ? r.note.slice(0, 40) : "";
  runtime.log(`  ${r.codec.padEnd(16)} ${status}  ${note}`);
}

runtime.log("");
runtime.log("all codec round-trips PASS");
