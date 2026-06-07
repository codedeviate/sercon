package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// streamEnableArgs builds `stream enable [--port <n>]`.
func streamEnableArgs(opts map[string]any) []string {
	args := []string{"stream", "enable"}
	if p, ok := opts["port"]; ok {
		args = append(args, "--port", fmt.Sprintf("%d", numToInt(p)))
	}
	return args
}

// chatArgs builds `chat <message> [--model <name>]`.
func chatArgs(message string, opts map[string]any) []string {
	args := []string{"chat", message}
	if m, _ := opts["model"].(string); m != "" {
		args = append(args, "--model", m)
	}
	return args
}

// batchArgs builds `batch [--bail] "<cmd>"...`. Each command is a full string.
func batchArgs(cmds []string, opts map[string]any) []string {
	args := []string{"batch"}
	if b, _ := opts["bail"].(bool); b {
		args = append(args, "--bail")
	}
	return append(args, cmds...)
}

func (h *abHandle) streamEnable(ctx context.Context, call goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, streamEnableArgs(optsArgMap(call, 0))...)
}
func (h *abHandle) streamDisable(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, "stream", "disable")
}
func (h *abHandle) streamStatus(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, "stream", "status")
}

func (h *abHandle) chat(ctx context.Context, call goja.FunctionCall) (any, error) {
	msg := strArg(call, 0)
	if msg == "" {
		return nil, errors.New("agentBrowser.chat: a message is required")
	}
	return h.runJSON(ctx, chatArgs(msg, optsArgMap(call, 1))...)
}

// cmd is the generic escape hatch: runs `agent-browser <command> <args...>`
// with --json/--session, returning the parsed envelope. Lets scripts reach
// agent-browser subcommands sercon doesn't model yet.
func (h *abHandle) cmd(ctx context.Context, call goja.FunctionCall) (any, error) {
	command := strArg(call, 0)
	if command == "" {
		return nil, errors.New("agentBrowser.cmd: a command is required")
	}
	args := []string{command}
	for i := 1; i < len(call.Arguments); i++ {
		args = append(args, strArg(call, i))
	}
	return h.runJSON(ctx, args...)
}

// batch runs multiple command strings sequentially. With --json the CLI emits
// a JSON array of per-command results, returned to the script as-is (an array,
// not the usual {success,data,error} envelope).
func (h *abHandle) batch(ctx context.Context, call goja.FunctionCall) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	raw, ok := call.Argument(0).Export().([]any)
	if !ok {
		return nil, errors.New("agentBrowser.batch: first argument must be an array of command strings")
	}
	cmds := make([]string, 0, len(raw))
	for _, c := range raw {
		cmds = append(cmds, fmt.Sprintf("%v", c))
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, batchArgs(cmds, optsArgMap(call, 1))...)
	if err != nil {
		return nil, err
	}
	// batch --json emits a top-level array; decode and return it directly.
	v, derr := scriptengine.DecodeOrderedJSON([]byte(out))
	if derr != nil {
		return parseJSON(out) // fall back to envelope wrapping
	}
	return v, nil
}

// addAdvanced wires stream/chat/cmd/batch into the handle object.
func (h *abHandle) addAdvanced(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["stream"] = map[string]any{
		"enable":  h.p(vm, loop, h.streamEnable),
		"disable": h.p(vm, loop, h.streamDisable),
		"status":  h.p(vm, loop, h.streamStatus),
	}
	obj["chat"] = h.p(vm, loop, h.chat)
	obj["cmd"] = h.p(vm, loop, h.cmd)
	obj["batch"] = h.p(vm, loop, h.batch)
}
