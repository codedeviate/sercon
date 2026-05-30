package main

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// serverNamespace builds the `server` top-level global. Today it exposes
// `server.http`, `server.https`, and `server.smtp`; future sub-specs will
// add `server.imap`, `server.ftp`. Each protocol's factory lives in its own
// api_server_<proto>.go file and returns a map[string]any of its members.
//
// The factory captures the per-Run engine pointer so bindings can call
// Engine.HoldRun to keep the loop alive while listeners are bound.
func serverNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"http":  httpServerMembers(vm, loop, eng, false),
		"https": httpServerMembers(vm, loop, eng, true),
		"smtp":  smtpServerMembers(vm, loop, eng),
		"tcp":   tcpServerMembers(vm, loop, eng),
	}
}
