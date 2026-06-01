// export default — the entry script's default export becomes Run's return
// value, which the CLI prints as JSON to stdout. Run: `sercon export-default.ts`
// prints the JSON object below, then the PASS line.

function summarize(items: number[]) {
  return { count: items.length, total: items.reduce((a, b) => a + b, 0) };
}

const result = summarize([2, 3, 5]);
runtime.log("computed:", result.count, "items");

export default result;
