package main

import (
	"testing"
)

// A finite float64 too large for int64 (|x| >= 2^63) must NOT take the int
// path: int64(x) is implementation-defined on overflow (saturates to
// MaxInt64 on arm64, MinInt64 on amd64), which would silently emit
// platform-dependent garbage. It must fall through to a float node instead.
func TestJSToIR_LargeFloatDoesNotOverflowToInt(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	for _, src := range []string{"1e300", "-1e300", "9.3e18" /* just over 2^63 */} {
		n, err := jsToIR(vm, mustEval(t, vm, src), opts)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if n.kind != dumpFloat {
			t.Errorf("%s: kind = %d, want dumpFloat (%d) — int path overflowed", src, n.kind, dumpFloat)
		}
	}
}

// Integral floats that DO fit int64 must still collapse to int (regression
// guard for the range gate).
func TestJSToIR_InRangeIntegralFloatStaysInt(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	for _, src := range []string{"3.0", "0.0", "-5.0", "9007199254740992" /* 2^53 */} {
		n, err := jsToIR(vm, mustEval(t, vm, src), opts)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if n.kind != dumpInt {
			t.Errorf("%s: kind = %d, want dumpInt (%d)", src, n.kind, dumpInt)
		}
	}
}
