package main

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// outStreamBinding builds the runtime.stdout / runtime.stderr handle for one
// stream. Registered per Run through the runtime namespace factory, so vm and
// loop are the live ones for this Run.
func outStreamBinding(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine, s *stream) map[string]any {
	return map[string]any{
		"to": func(call goja.FunctionCall) goja.Value {
			d, err := parseStreamTarget(vm, loop, e, call.Argument(0), teeOpt(call.Argument(1)))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return restoreFn(vm, s.push(d))
		},
		"toFile": func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			opts := call.Argument(1)
			d, err := fileDest(path, boolOpt(opts, "append"), boolOpt(opts, "tee"))
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("toFile: %w", err)))
			}
			return restoreFn(vm, s.push(d))
		},
		"silence": func(goja.FunctionCall) goja.Value {
			return restoreFn(vm, s.push(nullDest(false)))
		},
		"reset": func(goja.FunctionCall) goja.Value {
			s.reset()
			return goja.Undefined()
		},
		"target": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(s.targetInfo())
		},
	}
}

// restoreFn wraps a Go restore closure as a JS function. The closure is already
// idempotent (see stream.push), so a script may call it any number of times.
func restoreFn(vm *goja.Runtime, restore func()) goja.Value {
	return vm.ToValue(func(goja.FunctionCall) goja.Value {
		restore()
		return goja.Undefined()
	})
}

// teeOpt reads { tee: bool } from an options argument.
func teeOpt(v goja.Value) bool { return boolOpt(v, "tee") }

// boolOpt reads one boolean field from an options object, defaulting to false
// when the object or the field is absent.
func boolOpt(v goja.Value, name string) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return false
	}
	f := obj.Get(name)
	return f != nil && !goja.IsUndefined(f) && f.ToBoolean()
}

// parseStreamTarget turns a StreamTarget JS value into a destination.
//
//	"stdout" | "stderr"          -> fold onto that PROCESS stream
//	"null"                       -> discard
//	{ file, append? }            -> a file
//	(line: string) => void       -> a JS line handler
func parseStreamTarget(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine, target goja.Value, tee bool) (destination, error) {
	if target == nil || goja.IsUndefined(target) || goja.IsNull(target) {
		return destination{}, fmt.Errorf("to: a target is required (\"stdout\" | \"stderr\" | \"null\" | { file } | function)")
	}

	// A callable target is a line handler.
	if fn, ok := goja.AssertFunction(target); ok {
		return callbackDest(loop, e, fn, tee)
	}

	if obj, ok := target.(*goja.Object); ok {
		fileVal := obj.Get("file")
		if fileVal != nil && !goja.IsUndefined(fileVal) && !goja.IsNull(fileVal) {
			return fileDest(fileVal.String(), boolOpt(target, "append"), tee)
		}
		return destination{}, fmt.Errorf("to: object target needs a `file` property")
	}

	switch name := target.String(); name {
	case "null":
		if tee {
			return destination{}, fmt.Errorf("to: { tee: true } is meaningless with the \"null\" target — tee-ing to the void discards both copies")
		}
		return nullDest(false), nil
	case "stdout", "stderr":
		return processStreamDest(name, tee)
	default:
		return destination{}, fmt.Errorf("to: unknown target %q (want \"stdout\", \"stderr\", \"null\", { file }, or a function)", name)
	}
}
