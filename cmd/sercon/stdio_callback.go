package main

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// lineCallback is the destCallback payload. Task 4 implements it; this stub
// exists so the stream can reference the type from the start.
type lineCallback struct{}

func (c *lineCallback) tryFeed([]byte) bool { return false }
func (c *lineCallback) takePartial() []byte { return nil }
func (c *lineCallback) stop()               {}

// callbackDest builds a destCallback entry. Task 4 implements it.
func callbackDest(loop *eventloop.EventLoop, e *scriptengine.Engine, fn goja.Callable, tee bool) (destination, error) {
	return destination{}, fmt.Errorf("to: function targets are not implemented yet")
}
