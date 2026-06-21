package main

import (
	"fmt"
	"strings"
)

// cdpExec sends one CDP command through chromedriver's passthrough endpoint
// (POST /session/<id>/goog/cdp/execute) and returns the decoded result value.
// CDP is Chrome-only; firefox is rejected before any request is made. Callers
// must already hold s.do (it issues an s.command).
func (s *wdSession) cdpExec(cmd string, params map[string]any) (any, error) {
	if s.browser == "firefox" {
		return nil, fmt.Errorf("webdriver.cdp: CDP is Chrome-only (chromedriver); current browser is %q", s.browser)
	}
	if params == nil {
		params = map[string]any{}
	}
	v, err := s.command("POST", "/goog/cdp/execute", map[string]any{"cmd": cmd, "params": params})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown command") || strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "invalid session") {
			return nil, fmt.Errorf("%w (requires a CDP-capable chromedriver endpoint)", err)
		}
		return nil, err
	}
	return v, nil
}
