package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// command performs a raw, authenticated W3C WebDriver request to
// <baseURL>/session/<id><path> for endpoints tebeka/selenium v0.9.9 does not
// expose (actions, /window/new, /frame/parent, window rect, DELETE /window).
// It parses the standard { "value": ... } envelope: a non-2xx response or a
// value carrying error/message becomes a thrown error; otherwise the decoded
// value is returned (nil for a null/empty value). Callers must hold s.do.
//
//nolint:unused // wired by Phase 2 tasks (Task 2+); present here to keep the primitive in its own file
func (s *wdSession) command(method, path string, body any) (any, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	url := s.baseURL + "/session/" + s.wd.SessionID() + path
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env struct {
		Value json.RawMessage `json:"value"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &env)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("webdriver: %s %s: %s", method, path, wdEnvError(env.Value, resp.StatusCode))
	}
	if len(env.Value) == 0 || string(env.Value) == "null" {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal(env.Value, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// wdEnvError extracts a human-readable message from a W3C error `value`
// payload, falling back to the HTTP status when the payload is empty.
func wdEnvError(value json.RawMessage, status int) string {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if len(value) > 0 {
		_ = json.Unmarshal(value, &e)
	}
	switch {
	case e.Error != "" && e.Message != "":
		return e.Error + ": " + e.Message
	case e.Error != "":
		return e.Error
	case e.Message != "":
		return e.Message
	default:
		return fmt.Sprintf("HTTP %d", status)
	}
}

// toStringSlice converts a decoded JSON array of strings ([]any) to []string.
// Returns nil for any other input.
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
