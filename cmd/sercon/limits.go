package main

// Shared safety caps for parsers that consume untrusted external dumps
// (PHP serialize/var_dump/var_export, Perl Data::Dumper, and friends). These
// exist to turn pathological input (deeply nested structures, oversized
// counts, ...) into a normal returned error instead of a crash (stack
// overflow, OOM, panic).
const (
	// MaxDecodeDepth caps the nesting depth a recursive-descent decoder will
	// follow before giving up with an error. Well below the depth that would
	// exhaust a goroutine stack, but far beyond any realistic well-formed
	// dump.
	MaxDecodeDepth = 10_000
)
