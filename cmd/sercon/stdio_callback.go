package main

// lineCallback is the destCallback payload. Task 4 implements it; this stub
// exists so the stream can reference the type from the start.
type lineCallback struct{}

func (c *lineCallback) tryFeed([]byte) bool { return false }
func (c *lineCallback) takePartial() []byte { return nil }
func (c *lineCallback) stop()               {}
