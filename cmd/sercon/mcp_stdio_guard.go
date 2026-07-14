package main

import "os"

// mcpStdioGuard holds process-global state for the stdout->stderr redirect that
// keeps an MCP stdio server's stdout a pure JSON-RPC stream. The redirect is
// process-wide (it remaps fd 1), so a single active redirect is tracked here
// rather than per-mcpServer.
//
// Why the redirect is armed at mcp.serve() time, not srv.stdio() time: a script
// may write to stdout (console.log / runtime.log) BETWEEN building the server
// and calling stdio() — the stdio fixture does exactly that. Installing the
// redirect only inside stdio() would let those earlier writes land on the real
// stdout and corrupt the JSON-RPC stream. Arming at serve() moves them to
// stderr too. See installStdoutRedirect for why an fd-level remap (not an
// os.Stdout swap) is required.
//
// `armed` gates the serve()-time install so it engages ONLY for a real CLI run
// (runOne arms it around eng.RunFile). In-process engine tests drive eng.Run
// directly, never through runOne, so they leave `armed` false and keep their
// own stdout untouched.
var mcpStdioGuard struct {
	armed   bool
	real    *os.File     // saved real stdout for JSON-RPC while a redirect is active
	restore func() error // restores process stdout; nil when no redirect is active
}

// armMCPStdioGuard marks the current CLI run as eligible for the stdout
// redirect and returns a disarm func (call via defer): it clears the flag and,
// if a redirect was installed during the run and not already torn down, restores
// stdout.
func armMCPStdioGuard() func() {
	mcpStdioGuard.armed = true
	return func() {
		mcpStdioGuard.armed = false
		if mcpStdioGuard.restore != nil {
			_ = mcpStdioGuard.restore()
			mcpStdioGuard.restore = nil
			mcpStdioGuard.real = nil
		}
	}
}

// installMCPStdioRedirectIfArmed installs the redirect once, when armed. It is
// called from mcp.serve(). It is a no-op when not armed, or when a redirect is
// already active (returns the already-saved real stdout). On install failure it
// leaves the guard inactive and returns nil, deferring the error surface to
// stdio(), which retries and rejects its promise if the redirect still fails.
func installMCPStdioRedirectIfArmed() *os.File {
	if !mcpStdioGuard.armed || mcpStdioGuard.restore != nil {
		return mcpStdioGuard.real
	}
	real, restore, err := installStdoutRedirect()
	if err != nil {
		return nil
	}
	mcpStdioGuard.real, mcpStdioGuard.restore = real, restore
	return real
}
