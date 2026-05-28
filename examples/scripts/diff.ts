// Demonstrates api.text.diff.compare — unified diff between two text inputs.
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

const r = await api.text.diff.compare(before, after, {
  fromFile: "before.txt",
  toFile:   "after.txt",
  context:  2,
});

api.runtime.log("identical:", r.identical);
api.runtime.log("binary:   ", r.binary);
api.runtime.log("added:    ", r.added);
api.runtime.log("removed:  ", r.removed);
api.runtime.log("");
api.runtime.log("--- unified diff ---");
api.runtime.log(r.diff);

// Identical inputs short-circuit.
const same = await api.text.diff.compare("abc", "abc");
api.runtime.log("");
api.runtime.log("identical inputs:", same.identical, " diff length:", same.diff.length);

// Binary detection: an explicit NUL byte flips the binary flag.
const bin = await api.text.diff.compare("text", new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x00, 0x42]));
api.runtime.log("binary inputs:   ", bin.binary, " diff length:", bin.diff.length);
