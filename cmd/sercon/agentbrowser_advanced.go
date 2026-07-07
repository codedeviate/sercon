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

func (h *abHandle) streamEnable(ctx context.Context, opts map[string]any) (any, error) {
	return h.runJSON(ctx, streamEnableArgs(opts)...)
}
func (h *abHandle) streamDisable(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "stream", "disable")
}
func (h *abHandle) streamStatus(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "stream", "status")
}

// chatParams carries the message plus the chat options map.
type chatParams struct {
	msg  string
	opts map[string]any
}

func chatExtract(call goja.FunctionCall) (chatParams, error) {
	return chatParams{msg: strArg(call, 0), opts: optsArgMap(call, 1)}, nil
}

func (h *abHandle) chat(ctx context.Context, p chatParams) (any, error) {
	if p.msg == "" {
		return nil, errors.New("agentBrowser.chat: a message is required")
	}
	return h.runJSON(ctx, chatArgs(p.msg, p.opts)...)
}

// cmdExtract collects the command plus every trailing argument as strings.
func cmdExtract(call goja.FunctionCall) ([]string, error) {
	args := make([]string, 0, len(call.Arguments))
	for i := 0; i < len(call.Arguments); i++ {
		args = append(args, strArg(call, i))
	}
	return args, nil
}

// cmd is the generic escape hatch: runs `agent-browser <command> <args...>`
// with --json/--session, returning the parsed envelope. Lets scripts reach
// agent-browser subcommands sercon doesn't model yet.
func (h *abHandle) cmd(ctx context.Context, args []string) (any, error) {
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("agentBrowser.cmd: a command is required")
	}
	return h.runJSON(ctx, args...)
}

// batchParams carries the raw commands value plus the batch options map. cmds
// stays `any` so the array-type validation (and its error) lives in the work
// half, exactly as before.
type batchParams struct {
	cmds any
	opts map[string]any
}

func batchExtract(call goja.FunctionCall) (batchParams, error) {
	return batchParams{cmds: call.Argument(0).Export(), opts: optsArgMap(call, 1)}, nil
}

// batch runs multiple command strings sequentially. With --json the CLI emits
// a JSON array of per-command results, returned to the script as-is (an array,
// not the usual {success,data,error} envelope).
func (h *abHandle) batch(ctx context.Context, p batchParams) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	raw, ok := p.cmds.([]any)
	if !ok {
		return nil, errors.New("agentBrowser.batch: first argument must be an array of command strings")
	}
	cmds := make([]string, 0, len(raw))
	for _, c := range raw {
		cmds = append(cmds, fmt.Sprintf("%v", c))
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, batchArgs(cmds, p.opts)...)
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
		"enable":  abAsync(vm, loop, abOptsArg0, h.streamEnable),
		"disable": abAsync(vm, loop, abNoArgs, h.streamDisable),
		"status":  abAsync(vm, loop, abNoArgs, h.streamStatus),
	}
	obj["chat"] = abAsync(vm, loop, chatExtract, h.chat)
	obj["cmd"] = abAsync(vm, loop, cmdExtract, h.cmd)
	obj["batch"] = abAsync(vm, loop, batchExtract, h.batch)
}
