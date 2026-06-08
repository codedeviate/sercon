package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/tebeka/selenium"

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

// --- page ---

func (s *wdSession) addPage(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["source"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return s.wd.PageSource() })
	})
	obj["screenshot"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		path := strArg(call, 0)
		return s.do(func() (any, error) {
			data, err := s.wd.Screenshot()
			if err != nil {
				return nil, err
			}
			return wdDeliverShot(data, path)
		})
	})
}

// --- executeScript ---

func (s *wdSession) execScript(async bool) func(context.Context, goja.FunctionCall) (any, error) {
	return func(_ context.Context, call goja.FunctionCall) (any, error) {
		js := strArg(call, 0)
		if js == "" {
			return nil, errors.New("webdriver.executeScript: a script string is required")
		}
		var args []any
		if arr, ok := call.Argument(1).Export().([]any); ok {
			args = arr
		}
		return s.do(func() (any, error) {
			if async {
				return s.wd.ExecuteScriptAsync(js, args)
			}
			return s.wd.ExecuteScript(js, args)
		})
	}
}

func (s *wdSession) addScript(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["executeScript"] = wdAsync(vm, loop, s.execScript(false))
	obj["executeScriptAsync"] = wdAsync(vm, loop, s.execScript(true))
}

// --- cookies ---

// cookieFromMap builds a *selenium.Cookie from a JS object map.
// selenium.Cookie fields: Name, Value, Path, Domain string; Secure bool; Expiry uint.
// (HTTPOnly is absent in tebeka/selenium v0.9.9 — not set here.)
func cookieFromMap(m map[string]any) *selenium.Cookie {
	c := &selenium.Cookie{}
	c.Name, _ = m["name"].(string)
	c.Value, _ = m["value"].(string)
	c.Path, _ = m["path"].(string)
	c.Domain, _ = m["domain"].(string)
	c.Secure, _ = m["secure"].(bool)
	if exp, ok := m["expiry"]; ok {
		c.Expiry = uint(numToInt(exp))
	}
	return c
}

func (s *wdSession) addCookies(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["cookies"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return s.wd.GetCookies() })
	})
	obj["setCookie"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		m, ok := call.Argument(0).Export().(map[string]any)
		if !ok || m["name"] == nil {
			return nil, errors.New("webdriver.setCookie: a { name, value, ... } object is required")
		}
		return s.do(func() (any, error) { return wdOK(s.wd.AddCookie(cookieFromMap(m))) })
	})
	obj["deleteCookie"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		name := strArg(call, 0)
		if name == "" {
			return nil, errors.New("webdriver.deleteCookie: name is required")
		}
		return s.do(func() (any, error) { return wdOK(s.wd.DeleteCookie(name)) })
	})
	obj["deleteAllCookies"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return wdOK(s.wd.DeleteAllCookies()) })
	})
}

// --- waits ---

func (s *wdSession) addWaits(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["setImplicitWait"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		ms := numToInt(call.Argument(0).Export())
		return s.do(func() (any, error) {
			return wdOK(s.wd.SetImplicitWaitTimeout(time.Duration(ms) * time.Millisecond))
		})
	})
	obj["waitFor"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		by, value, err := findArgsWD(call)
		if err != nil {
			return nil, err
		}
		opts := optsArgMap(call, 2)
		timeout := 10000
		if t, ok := opts["timeout"]; ok {
			timeout = numToInt(t)
		}
		needVisible, _ := opts["visible"].(bool)
		deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
		for {
			res, derr := s.do(func() (any, error) {
				el, ferr := s.wd.FindElement(by, value)
				if ferr != nil {
					return nil, ferr
				}
				if needVisible {
					vis, verr := el.IsDisplayed()
					if verr != nil {
						return nil, verr
					}
					if !vis {
						return nil, errors.New("not visible")
					}
				}
				return s.elementObject(el, vm, loop), nil
			})
			if derr == nil {
				return res, nil
			}
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("webdriver.waitFor: %s=%q not found%s within %dms", by, value, visSuffix(needVisible), timeout)
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
}

func visSuffix(v bool) string {
	if v {
		return "/visible"
	}
	return ""
}

// wdFrameBody builds the W3C /frame request body from a switchToFrame target:
// a frame index (number) or an element handle (map carrying elementId). A
// string is rejected (tebeka would silently treat it as an element-id lookup).
func wdFrameBody(target any) (map[string]any, error) {
	switch v := target.(type) {
	case map[string]any:
		id, _ := v["elementId"].(string)
		if id == "" {
			return nil, errors.New("webdriver.switchToFrame: element handle has no elementId; pass an iframe element from find()")
		}
		return map[string]any{"id": map[string]string{webElementKey: id}}, nil
	case float64:
		return map[string]any{"id": int(v)}, nil
	case int64:
		return map[string]any{"id": int(v)}, nil
	case int:
		return map[string]any{"id": v}, nil
	default:
		return nil, errors.New("webdriver.switchToFrame: target must be a frame index (number) or an iframe element handle")
	}
}

// addFrames wires frame switching onto the session handle (all via W3C /frame).
func (s *wdSession) addFrames(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["switchToFrame"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		body, err := wdFrameBody(call.Argument(0).Export())
		if err != nil {
			return nil, err
		}
		return s.do(func() (any, error) { _, e := s.command("POST", "/frame", body); return wdOK(e) })
	})
	obj["switchToParentFrame"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { _, e := s.command("POST", "/frame/parent", map[string]any{}); return wdOK(e) })
	})
	obj["switchToDefaultContent"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { _, e := s.command("POST", "/frame", map[string]any{"id": nil}); return wdOK(e) })
	})
}

// wdWindowMethods names the window/tab methods on the session handle (used by
// tests to assert wiring).
var wdWindowMethods = map[string]bool{
	"windowHandles": true, "currentWindow": true, "switchToWindow": true,
	"newWindow": true, "closeWindow": true,
}

// addWindows wires window/tab management onto the session handle.
func (s *wdSession) addWindows(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["windowHandles"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return s.wd.WindowHandles() })
	})
	obj["currentWindow"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) { return s.wd.CurrentWindowHandle() })
	})
	obj["switchToWindow"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		h := strArg(call, 0)
		if h == "" {
			return nil, errors.New("webdriver.switchToWindow: a window handle is required")
		}
		return s.do(func() (any, error) { return wdOK(s.wd.SwitchWindow(h)) })
	})
	// newWindow uses the W3C POST /window/new (tebeka has no equivalent). type
	// is "tab" (default) or "window". Does not switch to the new window.
	obj["newWindow"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		typ := strArg(call, 0)
		if typ == "" {
			typ = "tab"
		}
		return s.do(func() (any, error) { return s.command("POST", "/window/new", map[string]any{"type": typ}) })
	})
	// closeWindow closes the current window via the W3C DELETE /window (which
	// returns the remaining handles) then auto-switches to a survivor, since
	// the browsing context is undefined after a close.
	obj["closeWindow"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		return s.do(func() (any, error) {
			v, err := s.command("DELETE", "/window", nil)
			if err != nil {
				return nil, err
			}
			remaining := toStringSlice(v)
			if len(remaining) > 0 {
				if err := s.wd.SwitchWindow(remaining[0]); err != nil {
					return nil, err
				}
			}
			return remaining, nil
		})
	})
}
