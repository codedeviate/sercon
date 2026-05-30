// Demonstrates codec.php.* and codec.perl.* — round-tripping JS values through
// PHP serialize / var_export / var_dump and Perl Data::Dumper.
//
// runtime.assert.equal does strict (reference) equality, so two distinct
// objects with identical contents are not "equal" to it. We assert the
// round-trips on a canonical JSON projection instead, which also documents
// that the decoded value is structurally identical to the original.
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

const order = { __class: "Order", id: 7, items: ["a", "b"], paid: true, note: null };

const s = codec.php.serialize(order);
runtime.log("serialize:    ", s);
runtime.assert.ok(same(codec.php.unserialize(s), order), "serialize round-trip");

const ve = codec.php.varExport(order);
runtime.log("var_export:\n" + ve);
runtime.assert.ok(same(codec.php.parseVarExport(ve), order), "var_export round-trip");

const vd = codec.php.varDump(order);
runtime.log("var_dump:\n" + vd);
runtime.assert.ok(same(codec.php.parseVarDump(vd), order), "var_dump round-trip");

// Perl: booleans use the JSON::XS::Boolean convention.
const flags = { active: true, archived: false, label: "vip" };
const pl = codec.perl.dumper(flags);
runtime.log("Data::Dumper:\n" + pl);
runtime.assert.ok(same(codec.perl.parseDumper(pl), flags), "Data::Dumper round-trip");

// Stable key order: serialize is byte-identical across runs (payment-hash safe).
runtime.assert.equal(codec.php.serialize(order), s, "serialize is deterministic");

runtime.log("\nOK — all four formats round-tripped.");
