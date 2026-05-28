// Demonstrates api.format.checkdigit.* — validate / compute / inspect across all
// supported algorithms: Luhn, ISBN-10, ISBN-13, EAN-13, EAN-8, UPC-A.

const samples: Array<[string, string]> = [
  ["luhn",   "4532015112830366"],  // Visa-style 16-digit
  ["luhn",   "79927398713"],        // Wikipedia Luhn vector
  ["isbn10", "0306406152"],
  ["isbn10", "048665088X"],         // ISBN-10 with "X" check
  ["isbn13", "9780306406157"],
  ["ean13",  "5901234123457"],
  ["ean8",   "73513537"],
  ["upca",   "036000291452"],
];

api.runtime.log("=== validate ===");
for (const [algo, code] of samples) {
  const ok = api.format.checkdigit.validate(algo, code);
  api.runtime.log(`  ${algo.padEnd(7)} ${code.padEnd(17)} ${ok ? "✓" : "✗"}`);
}

api.runtime.log("");
api.runtime.log("=== compute (reconstruct the last digit) ===");
for (const [algo, code] of samples) {
  const partial = code.slice(0, -1);
  const given = code.slice(-1);
  const computed = api.format.checkdigit.compute(algo, partial);
  const ok = computed.toUpperCase() === given.toUpperCase();
  api.runtime.log(`  ${algo.padEnd(7)} ${partial.padEnd(16)} -> ${computed}  ${ok ? "✓" : "✗"} (given ${given})`);
}

api.runtime.log("");
api.runtime.log("=== inspect (single-shot report) ===");
const r = api.format.checkdigit.inspect("luhn", "4532015112830366");
api.runtime.log("  algo:    ", r.algo);
api.runtime.log("  input:   ", r.input);
api.runtime.log("  given:   ", r.given);
api.runtime.log("  computed:", r.computed);
api.runtime.log("  valid:   ", r.valid);

// A bad code surfaces as valid:false
const bad = api.format.checkdigit.inspect("luhn", "4532015112830367");
api.runtime.log("");
api.runtime.log("=== inspect of an invalid Luhn ===");
api.runtime.log("  valid:   ", bad.valid, " given:", bad.given, " computed:", bad.computed);

api.runtime.log("");
api.runtime.log("supported algorithms:", api.format.checkdigit.algos().join(", "));
