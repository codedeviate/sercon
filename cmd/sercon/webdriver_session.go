package main

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// wdNavMethods documents the nav method names exposed on the session handle.
// Also used by tests to assert all nav bindings are wired.
var wdNavMethods = map[string]bool{
	"get":     true,
	"url":     true,
	"title":   true,
	"back":    true,
	"forward": true,
	"refresh": true,
}

// addNav wires the navigation methods onto the session handle object.
func (s *wdSession) addNav(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["get"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		url := strArg(call, 0)
		if url == "" {
			return nil, errors.New("webdriver.get: url is required")
		}
		return s.do(func() (any, error) { return wdOK(s.wd.Get(url)) })
	})
	obj["url"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return s.wd.CurrentURL() })
	})
	obj["title"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return s.wd.Title() })
	})
	obj["back"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return wdOK(s.wd.Back()) })
	})
	obj["forward"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return wdOK(s.wd.Forward()) })
	})
	obj["refresh"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return wdOK(s.wd.Refresh()) })
	})
}

// wdOK turns a void WebDriver call into a JS-friendly { ok: true } result,
// propagating any error.
func wdOK(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	o := scriptengine.NewOrdered()
	o.Set("ok", true)
	return o, nil
}
