package main

import "github.com/dop251/goja"

// phpNamespace wires codec.php.* — read/write PHP serialize / var_export /
// var_dump. All members are synchronous; malformed input throws.
func phpNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }

	encodeWith := func(fn func(*irNode, dumpOpts) (string, error)) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			opts := dumpOptsFromArg(vm, call.Argument(1))
			n, err := jsToIR(vm, call.Argument(0), opts)
			if err != nil {
				return throw(err)
			}
			s, err := fn(n, opts)
			if err != nil {
				return throw(err)
			}
			return vm.ToValue(s)
		}
	}
	decodeWith := func(fn func(string, dumpOpts) (*irNode, error)) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			opts := dumpOptsFromArg(vm, call.Argument(1))
			n, err := fn(call.Argument(0).String(), opts)
			if err != nil {
				return throw(err)
			}
			return irToJS(vm, n, opts)
		}
	}

	return map[string]any{
		"serialize":      encodeWith(phpSerializeEncode),
		"unserialize":    decodeWith(phpSerializeDecode),
		"varExport":      encodeWith(phpVarExportEncode),
		"parseVarExport": decodeWith(phpVarExportDecode),
		"varDump":        encodeWith(phpVarDumpEncode),
		"parseVarDump":   decodeWith(phpVarDumpDecode),
	}
}

// perlNamespace wires codec.perl.* — read/write Perl Data::Dumper.
func perlNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"dumper": func(call goja.FunctionCall) goja.Value {
			opts := dumpOptsFromArg(vm, call.Argument(1))
			n, err := jsToIR(vm, call.Argument(0), opts)
			if err != nil {
				return throw(err)
			}
			s, err := perlDumperEncode(n, opts)
			if err != nil {
				return throw(err)
			}
			return vm.ToValue(s)
		},
		"parseDumper": func(call goja.FunctionCall) goja.Value {
			opts := dumpOptsFromArg(vm, call.Argument(1))
			n, err := perlDumperDecode(call.Argument(0).String(), opts)
			if err != nil {
				return throw(err)
			}
			return irToJS(vm, n, opts)
		},
	}
}

// dumpOptsFromArg reads an optional { classKey?, perlBoolClass?, indent? }
// options object from a JS argument, then fills defaults via withDumpDefaults.
// A missing/undefined/non-object argument yields the defaults.
func dumpOptsFromArg(vm *goja.Runtime, arg goja.Value) dumpOpts {
	var o dumpOpts
	if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		if obj, ok := arg.(*goja.Object); ok {
			if v := obj.Get("classKey"); v != nil && !goja.IsUndefined(v) {
				o.classKey = v.String()
			}
			if v := obj.Get("perlBoolClass"); v != nil && !goja.IsUndefined(v) {
				o.perlBoolClass = v.String()
			}
			if v := obj.Get("indent"); v != nil && !goja.IsUndefined(v) {
				o.indent = v.String()
			}
		}
	}
	return withDumpDefaults(o)
}
