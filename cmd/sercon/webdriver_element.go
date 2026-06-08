package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/tebeka/selenium"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// webElementKey is the W3C element-reference JSON key.
const webElementKey = "element-6066-11e4-a52e-4f735466cecf"

// wdElementID returns the W3C element id for el via tebeka's JSON marshalling
// (remoteWE.MarshalJSON emits both ELEMENT and element-6066… keys).
func wdElementID(el selenium.WebElement) string {
	b, err := json.Marshal(el)
	if err != nil {
		return ""
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m[webElementKey]
}

// wdDeliverShot returns a screenshot result: { path, size, format } when a path
// is given (writes the PNG), else { bytes: []byte, format } in memory.
func wdDeliverShot(data []byte, userPath string) (any, error) {
	o := scriptengine.NewOrdered()
	if userPath != "" {
		if err := os.WriteFile(userPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("webdriver: writing screenshot to %s: %w", userPath, err)
		}
		o.Set("path", userPath)
		o.Set("size", len(data))
		o.Set("format", "png")
		return o, nil
	}
	o.Set("bytes", data) // []byte -> JS number[]
	o.Set("format", "png")
	return o, nil
}

// findArgsWD reads (by, value) from a call and resolves the strategy.
func findArgsWD(call goja.FunctionCall) (by string, value string, err error) {
	byStr := strArg(call, 0)
	value = strArg(call, 1)
	if byStr == "" || value == "" {
		return "", "", errors.New("webdriver.find: (by, value) are required")
	}
	by, err = byStrategy(byStr)
	return by, value, err
}

// elementObject builds the goja-facing element handle wrapping el. All calls
// funnel through the session mutex (s.do) — element ops share the session.
func (s *wdSession) elementObject(el selenium.WebElement, vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	str := func(fn func() (string, error)) func(context.Context, goja.FunctionCall) (any, error) {
		return func(_ context.Context, _ goja.FunctionCall) (any, error) {
			return s.do(func() (any, error) { return fn() })
		}
	}
	boolFn := func(fn func() (bool, error)) func(context.Context, goja.FunctionCall) (any, error) {
		return func(_ context.Context, _ goja.FunctionCall) (any, error) {
			return s.do(func() (any, error) { return fn() })
		}
	}
	eid := wdElementID(el)
	return map[string]any{
		"click": wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
			return s.do(func() (any, error) { return wdOK(el.Click()) })
		}),
		"clear": wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
			return s.do(func() (any, error) { return wdOK(el.Clear()) })
		}),
		"submit": wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
			return s.do(func() (any, error) { return wdOK(el.Submit()) })
		}),
		"sendKeys": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			text := strArg(call, 0)
			return s.do(func() (any, error) { return wdOK(el.SendKeys(text)) })
		}),
		"text":    wdAsync(vm, loop, str(el.Text)),
		"tagName": wdAsync(vm, loop, str(el.TagName)),
		"getAttribute": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			name := strArg(call, 0)
			if name == "" {
				return nil, errors.New("webdriver.getAttribute: name is required")
			}
			return s.do(func() (any, error) {
				v, err := el.GetAttribute(name)
				// W3C "Get Element Attribute" returns null for an absent
				// attribute; tebeka surfaces that JSON null as the sentinel
				// error "nil return value". Map it back to JS null instead of
				// throwing. (geckodriver returns null for attributes Chrome
				// exposes as live properties, e.g. an input's typed `value` —
				// read those via executeScript("return arguments[0].value", [el]).)
				if err != nil && err.Error() == "nil return value" {
					return nil, nil
				}
				return v, err
			})
		}),
		"cssValue": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			name := strArg(call, 0)
			if name == "" {
				return nil, errors.New("webdriver.cssValue: name is required")
			}
			return s.do(func() (any, error) { return el.CSSProperty(name) })
		}),
		"isDisplayed": wdAsync(vm, loop, boolFn(el.IsDisplayed)),
		"isEnabled":   wdAsync(vm, loop, boolFn(el.IsEnabled)),
		"isSelected":  wdAsync(vm, loop, boolFn(el.IsSelected)),
		"find": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			by, value, err := findArgsWD(call)
			if err != nil {
				return nil, err
			}
			return s.do(func() (any, error) {
				child, err := el.FindElement(by, value)
				if err != nil {
					return nil, err
				}
				return s.elementObject(child, vm, loop), nil
			})
		}),
		"findAll": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			by, value, err := findArgsWD(call)
			if err != nil {
				return nil, err
			}
			return s.do(func() (any, error) {
				kids, err := el.FindElements(by, value)
				if err != nil {
					return nil, err
				}
				out := make([]any, 0, len(kids))
				for _, k := range kids {
					out = append(out, s.elementObject(k, vm, loop))
				}
				return out, nil
			})
		}),
		"screenshot": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			path := strArg(call, 0)
			return s.do(func() (any, error) {
				data, err := el.Screenshot(true)
				if err != nil {
					return nil, err
				}
				return wdDeliverShot(data, path)
			})
		}),
		"elementId": eid,
		"hover": wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
			return s.do(func() (any, error) { return s.hoverElement(eid) })
		}),
		"dragTo": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			dst, err := wdElementIDArg(call, 0)
			if err != nil {
				return nil, err
			}
			return s.do(func() (any, error) { return s.dragElement(eid, dst) })
		}),
	}
}

// addFind wires session-level find/findAll (returning element handles).
func (s *wdSession) addFind(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["find"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		by, value, err := findArgsWD(call)
		if err != nil {
			return nil, err
		}
		return s.do(func() (any, error) {
			el, err := s.wd.FindElement(by, value)
			if err != nil {
				return nil, err
			}
			return s.elementObject(el, vm, loop), nil
		})
	})
	obj["findAll"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		by, value, err := findArgsWD(call)
		if err != nil {
			return nil, err
		}
		return s.do(func() (any, error) {
			els, err := s.wd.FindElements(by, value)
			if err != nil {
				return nil, err
			}
			out := make([]any, 0, len(els))
			for _, el := range els {
				out = append(out, s.elementObject(el, vm, loop))
			}
			return out, nil
		})
	})
}
