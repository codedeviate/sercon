package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/oauth2"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// jsOAuthHandler adapts a script's getToken callback to auth.OAuthHandler.
// getToken returns a bearer-token string (sync or async); it is invoked via the
// on-loop callJSHandler bridge from the transport goroutine.
type jsOAuthHandler struct {
	loop     *eventloop.EventLoop
	getToken *scriptengine.LoopCallable
}

// jsTokenSource calls getToken each time a token is needed. The transport wraps
// requests with this; Authorize (below) forces a fresh fetch on 401/403.
type jsTokenSource struct {
	loop     *eventloop.EventLoop
	getToken *scriptengine.LoopCallable
}

func (ts jsTokenSource) Token() (*oauth2.Token, error) {
	out, err := callJSHandler(ts.loop, ts.getToken,
		func(vm *goja.Runtime) []goja.Value { return nil },
		func(vm *goja.Runtime, v goja.Value) (any, error) {
			if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
				return "", nil
			}
			return v.String(), nil
		})
	if err != nil {
		return nil, err
	}
	tok, ok := out.(string)
	if !ok || tok == "" {
		return nil, fmt.Errorf("mcp.connect: auth.getToken must return a non-empty token string")
	}
	return &oauth2.Token{AccessToken: tok, TokenType: "Bearer"}, nil
}

func (h jsOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return jsTokenSource(h), nil
}

// Authorize is invoked on a 401/403. Close the response body and return nil so
// the transport re-derives the token via TokenSource().Token() (which re-calls
// getToken — the script should return a refreshed token) and retries.
func (h jsOAuthHandler) Authorize(_ context.Context, _ *http.Request, resp *http.Response) error {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return nil
}
