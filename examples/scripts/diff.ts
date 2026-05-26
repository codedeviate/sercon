// Demonstrates api.diff.compare — unified diff between two text inputs.
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

const r = await api.diff.compare(before, after, {
  fromFile: "before.txt",
  toFile:   "after.txt",
  context:  2,
});

api.log("identical:", r.identical);
api.log("binary:   ", r.binary);
api.log("added:    ", r.added);
api.log("removed:  ", r.removed);
api.log("");
api.log("--- unified diff ---");
api.log(r.diff);

// Identical inputs short-circuit.
const same = await api.diff.compare("abc", "abc");
api.log("");
api.log("identical inputs:", same.identical, " diff length:", same.diff.length);

// Binary detection: an explicit NUL byte flips the binary flag.
const bin = await api.diff.compare("text", new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x00, 0x42]));
api.log("binary inputs:   ", bin.binary, " diff length:", bin.diff.length);
