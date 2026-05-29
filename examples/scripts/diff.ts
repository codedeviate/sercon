// Demonstrates text.diff.compare — unified diff between two text inputs.
// Returns { identical, binary, added, removed, diff, format }. Binary
// inputs (NUL byte in the first 8 KB) short-circuit with binary:true and
// no diff text.

const before = `one
two
three
four
five
`;

const after = `one
two-edited
three
five
six
`;

const r = await text.diff.compare(before, after, {
  fromFile: "before.txt",
  toFile:   "after.txt",
  context:  2,
});

runtime.log("identical:", r.identical);
runtime.log("binary:   ", r.binary);
runtime.log("added:    ", r.added);
runtime.log("removed:  ", r.removed);
runtime.log("");
runtime.log("--- unified diff ---");
runtime.log(r.diff);

// Identical inputs short-circuit.
const same = await text.diff.compare("abc", "abc");
runtime.log("");
runtime.log("identical inputs:", same.identical, " diff length:", same.diff.length);

// Binary detection: an explicit NUL byte flips the binary flag.
const bin = await text.diff.compare("text", new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x00, 0x42]));
runtime.log("binary inputs:   ", bin.binary, " diff length:", bin.diff.length);
