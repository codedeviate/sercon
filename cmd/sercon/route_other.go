//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package main

import (
	"fmt"
	"runtime"
)

// hostRoutes is unsupported off Linux/BSD/macOS. The portable pure-Go paths
// are procfs (Linux) and the routing socket via x/net/route (BSD/macOS);
// Windows would need the IP Helper API, deliberately out of scope here. The
// stub keeps release cross-compiles green and throws a clean error.
func hostRoutes() ([]routeEntry, error) {
	return nil, fmt.Errorf("route enumeration not supported on %s (Linux/macOS/BSD only)", runtime.GOOS)
}
