package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// Pointer input note: empirically (chromedriver 149), pointerMove actions with
// an *element-origin* are accepted by the driver but do NOT dispatch DOM mouse
// events in this binding's session context, whereas *viewport-coordinate*
// moves do. So hover/drag resolve the element's centre via its rect and move
// the pointer in viewport coordinates. Each sequence also starts from an
// off-element anchor so the move onto the target is always a non-zero-distance
// move (the W3C virtual pointer is stateful — a repeat move to the same point
// emits nothing). Callers that want element-origin or other shapes can use the
// performActions escape hatch with a raw sequence.

// anchorOffset is the vertical gap (px) between the off-element anchor and the
// target centre, ensuring the move onto the target crosses its boundary.
const anchorOffset = 200

// wdMouseSeq wraps a list of pointer actions in a mouse input source.
func wdMouseSeq(actions ...any) map[string]any {
	return map[string]any{"actions": []any{
		map[string]any{
			"type":       "pointer",
			"id":         "mouse",
			"parameters": map[string]any{"pointerType": "mouse"},
			"actions":    actions,
		},
	}}
}

// wdHoverViewport builds the /actions body for a viewport-coordinate hover:
// anchor below the target centre, then move onto it.
func wdHoverViewport(cx, cy int) map[string]any {
	return wdMouseSeq(
		map[string]any{"type": "pointerMove", "duration": 0, "origin": "viewport", "x": cx, "y": cy + anchorOffset},
		map[string]any{"type": "pointerMove", "duration": 0, "origin": "viewport", "x": cx, "y": cy},
	)
}

// wdDragViewport builds the /actions body for a viewport-coordinate drag:
// anchor below src, move onto src, press, move onto dst, release.
func wdDragViewport(srcX, srcY, dstX, dstY int) map[string]any {
	return wdMouseSeq(
		map[string]any{"type": "pointerMove", "duration": 0, "origin": "viewport", "x": srcX, "y": srcY + anchorOffset},
		map[string]any{"type": "pointerMove", "duration": 0, "origin": "viewport", "x": srcX, "y": srcY},
		map[string]any{"type": "pointerDown", "button": 0},
		map[string]any{"type": "pointerMove", "duration": 100, "origin": "viewport", "x": dstX, "y": dstY},
		map[string]any{"type": "pointerUp", "button": 0},
	)
}

// wdKeyChordActions presses keys down in order, then releases in reverse order
// (so modifiers wrap the inner key), e.g. ["Control","a"] -> Ctrl+A.
func wdKeyChordActions(keys []string) map[string]any {
	acts := make([]any, 0, len(keys)*2)
	for _, k := range keys {
		acts = append(acts, map[string]any{"type": "keyDown", "value": k})
	}
	for i := len(keys) - 1; i >= 0; i-- {
		acts = append(acts, map[string]any{"type": "keyUp", "value": keys[i]})
	}
	return map[string]any{"actions": []any{
		map[string]any{"type": "key", "id": "keyboard", "actions": acts},
	}}
}

// wdElementCenter fetches the W3C element rect and returns the element's
// viewport-centre coordinates. Callers must hold s.do (it issues an s.command).
func (s *wdSession) wdElementCenter(eid string) (cx, cy int, err error) {
	v, err := s.command("GET", "/element/"+eid+"/rect", nil)
	if err != nil {
		return 0, 0, fmt.Errorf("webdriver: element rect: %w", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return 0, 0, errors.New("webdriver: element rect: unexpected response")
	}
	x, _ := m["x"].(float64)
	y, _ := m["y"].(float64)
	w, _ := m["width"].(float64)
	h, _ := m["height"].(float64)
	return int(x + w/2), int(y + h/2), nil
}

// wdElementIDArg reads an element-handle argument and returns its W3C element id.
func wdElementIDArg(call goja.FunctionCall, i int) (string, error) {
	m, ok := call.Argument(i).Export().(map[string]any)
	if !ok {
		return "", fmt.Errorf("webdriver: argument %d must be an element handle", i+1)
	}
	id, _ := m["elementId"].(string)
	if id == "" {
		return "", fmt.Errorf("webdriver: argument %d is not a valid element handle (no elementId)", i+1)
	}
	return id, nil
}

// hoverElement moves the pointer over the element with the given id.
func (s *wdSession) hoverElement(eid string) (any, error) {
	cx, cy, err := s.wdElementCenter(eid)
	if err != nil {
		return nil, err
	}
	_, e := s.command("POST", "/actions", wdHoverViewport(cx, cy))
	return wdOK(e)
}

// dragElement drags from the src element onto the dst element.
func (s *wdSession) dragElement(srcID, dstID string) (any, error) {
	sx, sy, err := s.wdElementCenter(srcID)
	if err != nil {
		return nil, err
	}
	dx, dy, err := s.wdElementCenter(dstID)
	if err != nil {
		return nil, err
	}
	_, e := s.command("POST", "/actions", wdDragViewport(sx, sy, dx, dy))
	return wdOK(e)
}

// dragAndDropArgs carries the on-loop-extracted (src, dst) element ids for
// the dragAndDrop binding.
type dragAndDropArgs struct {
	src, dst string
}

// addActions wires the W3C action-chain methods onto the session handle.
func (s *wdSession) addActions(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["hover"] = wdAsync(vm, loop, func(call goja.FunctionCall) (string, error) {
		return wdElementIDArg(call, 0)
	}, func(_ context.Context, id string) (any, error) {
		return s.do(func() (any, error) { return s.hoverElement(id) })
	})
	obj["dragAndDrop"] = wdAsync(vm, loop, func(call goja.FunctionCall) (dragAndDropArgs, error) {
		src, err := wdElementIDArg(call, 0)
		if err != nil {
			return dragAndDropArgs{}, err
		}
		dst, err := wdElementIDArg(call, 1)
		if err != nil {
			return dragAndDropArgs{}, err
		}
		return dragAndDropArgs{src: src, dst: dst}, nil
	}, func(_ context.Context, a dragAndDropArgs) (any, error) {
		return s.do(func() (any, error) { return s.dragElement(a.src, a.dst) })
	})
	obj["keyChord"] = wdAsync(vm, loop, func(call goja.FunctionCall) ([]string, error) {
		keys := make([]string, 0, len(call.Arguments))
		for i := range call.Arguments {
			keys = append(keys, strArg(call, i))
		}
		if len(keys) == 0 {
			return nil, errors.New("webdriver.keyChord: at least one key is required")
		}
		return keys, nil
	}, func(_ context.Context, keys []string) (any, error) {
		return s.do(func() (any, error) { _, e := s.command("POST", "/actions", wdKeyChordActions(keys)); return wdOK(e) })
	})
	obj["performActions"] = wdAsync(vm, loop, func(call goja.FunctionCall) ([]any, error) {
		seq, ok := call.Argument(0).Export().([]any)
		if !ok {
			return nil, errors.New("webdriver.performActions: argument must be an array of W3C action sequences")
		}
		return seq, nil
	}, func(_ context.Context, seq []any) (any, error) {
		return s.do(func() (any, error) {
			_, e := s.command("POST", "/actions", map[string]any{"actions": seq})
			return wdOK(e)
		})
	})
	obj["releaseActions"] = wdAsync(vm, loop, wdNoArgs, func(_ context.Context, _ struct{}) (any, error) {
		return s.do(func() (any, error) { _, e := s.command("DELETE", "/actions", nil); return wdOK(e) })
	})
}
