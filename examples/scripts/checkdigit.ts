// Demonstrates api.checkdigit.* — validate / compute / inspect across all
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

api.log("=== validate ===");
for (const [algo, code] of samples) {
  const ok = api.checkdigit.validate(algo, code);
  api.log(`  ${algo.padEnd(7)} ${code.padEnd(17)} ${ok ? "✓" : "✗"}`);
}

api.log("");
api.log("=== compute (reconstruct the last digit) ===");
for (const [algo, code] of samples) {
  const partial = code.slice(0, -1);
  const given = code.slice(-1);
  const computed = api.checkdigit.compute(algo, partial);
  const ok = computed.toUpperCase() === given.toUpperCase();
  api.log(`  ${algo.padEnd(7)} ${partial.padEnd(16)} -> ${computed}  ${ok ? "✓" : "✗"} (given ${given})`);
}

api.log("");
api.log("=== inspect (single-shot report) ===");
const r = api.checkdigit.inspect("luhn", "4532015112830366");
api.log("  algo:    ", r.algo);
api.log("  input:   ", r.input);
api.log("  given:   ", r.given);
api.log("  computed:", r.computed);
api.log("  valid:   ", r.valid);

// A bad code surfaces as valid:false
const bad = api.checkdigit.inspect("luhn", "4532015112830367");
api.log("");
api.log("=== inspect of an invalid Luhn ===");
api.log("  valid:   ", bad.valid, " given:", bad.given, " computed:", bad.computed);

api.log("");
api.log("supported algorithms:", api.checkdigit.algos().join(", "));
