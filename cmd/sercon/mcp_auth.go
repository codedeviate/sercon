package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dop251/goja"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcp_auth.go implements the optional OAuth 2.0 protected-resource-server layer
// for srv.listen({ auth }) (Streamable HTTP transport, Phase 3 Task 5).
//
// A resource server (RS) in the OAuth model does two things, both implemented
// here: (1) it rejects requests that don't carry a valid bearer token — via the
// SDK's auth.RequireBearerToken middleware, which turns a verifier error into a
// 401 with a WWW-Authenticate header — and (2) it advertises where clients
// should go to obtain a token, by serving RFC 9728 protected-resource metadata
// at /.well-known/oauth-protected-resource.
//
// Explicitly OUT OF SCOPE: dynamic client registration (DCR, RFC 7591) and any
// token *issuance*. Those are the AUTHORIZATION server's job; a resource server
// never mints or registers tokens — it only validates them and points clients
// at the authorization server(s) via the `authorization_servers` metadata
// field. sercon supplies the validation logic entirely through the script's
// `verify` callback (the script decides what a valid token means), so sercon is
// a pure RS with no embedded AS.

// mcpAuthConfig is the parsed, validated `auth` block from listen opts. It is
// nil when the script passed no `auth` (the unauthenticated Phase-1 path).
type mcpAuthConfig struct {
	// verify is the script's (token, reqInfo) => identity|null callback, wrapped
	// so it can be invoked from an HTTP-server goroutine and marshalled onto the
	// event loop. Required.
	verify *scriptengine.LoopCallable
	// scopes are the scopes RequireBearerToken enforces on every request (all
	// must be present on the verified token) and echoes into WWW-Authenticate.
	scopes []string
	// meta is the protected-resource metadata served at the well-known path.
	// Resource is filled in later (in jsListen) once the bound address is known.
	meta *oauthex.ProtectedResourceMetadata
}

// mcpProtectedResourcePath is the RFC 9728 well-known location for
// protected-resource metadata.
const mcpProtectedResourcePath = "/.well-known/oauth-protected-resource"

// mcpDefaultTokenTTL is the assumed lifetime applied to a verified token when
// the script's identity omits `expiresAt`. auth.RequireBearerToken treats a
// zero Expiration as an invalid token (it 401s on Expiration.IsZero()), so an
// identity without an explicit expiry must still carry *some* future instant.
// One hour is a conservative default; scripts that mint short- or long-lived
// sessions should set `expiresAt` explicitly.
const mcpDefaultTokenTTL = time.Hour

// parseMCPAuth extracts and validates the optional `auth` block from a
// listen({ ... }) options object. It returns nil when `auth` is absent (the
// unauthenticated path). It panics with a vm TypeError when `auth` is present
// but malformed (most importantly, `verify` not being a function) — matching
// jsListen's synchronous, pre-bind validation style so a bad config never
// leaves a half-bound listener behind.
func (ms *mcpServer) parseMCPAuth(optsObj *goja.Object) *mcpAuthConfig {
	av := optsObj.Get("auth")
	if av == nil || goja.IsUndefined(av) || goja.IsNull(av) {
		return nil
	}
	authObj := av.ToObject(ms.vm)

	verifyVal := authObj.Get("verify")
	verifyFn, ok := goja.AssertFunction(verifyVal)
	if !ok {
		panic(ms.vm.NewTypeError("mcp: listen: auth.verify must be a function"))
	}

	cfg := &mcpAuthConfig{
		verify: scriptengine.NewLoopCallable(ms.loop, verifyFn),
		meta:   &oauthex.ProtectedResourceMetadata{},
	}
	cfg.scopes = stringSliceArg(ms.vm, authObj.Get("scopes"))

	if rmVal := authObj.Get("resourceMetadata"); rmVal != nil && !goja.IsUndefined(rmVal) && !goja.IsNull(rmVal) {
		rm := rmVal.ToObject(ms.vm)
		cfg.meta.AuthorizationServers = stringSliceArg(ms.vm, rm.Get("authorizationServers"))
		cfg.meta.ScopesSupported = stringSliceArg(ms.vm, rm.Get("scopesSupported"))
		if v := rm.Get("resourceName"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			cfg.meta.ResourceName = v.String()
		}
		if v := rm.Get("resourceDocumentation"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			cfg.meta.ResourceDocumentation = v.String()
		}
		if v := rm.Get("jwksUri"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			cfg.meta.JWKSURI = v.String()
		}
		if v := rm.Get("bearerMethodsSupported"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			cfg.meta.BearerMethodsSupported = stringSliceArg(ms.vm, v)
		}
		// `resource` override — otherwise jsListen derives it from the bound
		// base URL (see finalizeMCPAuthMeta).
		if v := rm.Get("resource"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			cfg.meta.Resource = v.String()
		}
	}

	// Fall back to the enforced scopes for advertised scopes when the script
	// didn't spell out scopesSupported but did require scopes.
	if len(cfg.meta.ScopesSupported) == 0 && len(cfg.scopes) > 0 {
		cfg.meta.ScopesSupported = cfg.scopes
	}
	return cfg
}

// stringSliceArg coerces a goja value that should be a string[] into a Go
// []string, tolerating absent/undefined/null (→ nil). Non-array values throw.
func stringSliceArg(vm *goja.Runtime, v goja.Value) []string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	arr, ok := v.Export().([]any)
	if !ok {
		panic(vm.NewTypeError("mcp: listen: auth string-list fields must be arrays of strings"))
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			panic(vm.NewTypeError("mcp: listen: auth string-list fields must contain only strings"))
		}
		out = append(out, s)
	}
	return out
}

// finalizeMCPAuthMeta fills the metadata's Resource identifier from the bound
// base URL when the script didn't supply an explicit `resource` override. Per
// RFC 9728 the resource identifier is the RS's own URL; we use the server's
// base URL (scheme://host:port).
func (cfg *mcpAuthConfig) finalizeMCPAuthMeta(baseURL string) {
	if cfg.meta.Resource == "" {
		cfg.meta.Resource = baseURL
	}
}

// tokenIdentity is the native-Go form of the script's verify() return value,
// extracted from goja WHILE STILL ON THE LOOP (in the callJSHandler convert
// callback). Nothing goja-owned escapes the loop goroutine.
type tokenIdentity struct {
	subject   string
	scopes    []string
	expiresAt time.Time
	hasExpiry bool
	extra     map[string]any
}

// toTokenInfo builds the SDK's *auth.TokenInfo. Expiration is mandatory to the
// RequireBearerToken middleware, so an identity without an explicit expiry gets
// the default TTL (see mcpDefaultTokenTTL).
func (id *tokenIdentity) toTokenInfo() *auth.TokenInfo {
	exp := id.expiresAt
	if !id.hasExpiry {
		exp = time.Now().Add(mcpDefaultTokenTTL)
	}
	return &auth.TokenInfo{
		UserID:     id.subject,
		Scopes:     id.scopes,
		Expiration: exp,
		Extra:      id.extra,
	}
}

// tokenVerifier adapts the script's verify() callback into an
// auth.TokenVerifier. RequireBearerToken calls this synchronously, per request,
// on an HTTP-server goroutine — NEVER the event loop's goroutine — so every
// goja access is funnelled through callJSHandler, which schedules the call onto
// the loop and blocks this goroutine until it settles, converting the result to
// native Go data on-loop. That is the same discipline the MCP tool/resource/
// prompt handlers use; it is what keeps goja single-threaded here.
//
// Mapping: a null/undefined return (or a thrown/rejected verify) → an error
// wrapping auth.ErrInvalidToken, which the middleware renders as a 401. A
// returned object → the verified identity.
func (ms *mcpServer) tokenVerifier(cfg *mcpAuthConfig) auth.TokenVerifier {
	return func(ctx context.Context, token string, req *http.Request) (*auth.TokenInfo, error) {
		method := req.Method
		path := req.URL.Path
		header := map[string]any{}
		for k := range req.Header {
			header[k] = req.Header.Get(k)
		}

		out, err := ms.callJSHandler(
			cfg.verify,
			func(vm *goja.Runtime) []goja.Value {
				reqInfo := vm.NewObject()
				_ = reqInfo.Set("method", method)
				_ = reqInfo.Set("path", path)
				_ = reqInfo.Set("header", header)
				return []goja.Value{vm.ToValue(token), reqInfo}
			},
			convertTokenIdentity,
		)
		if err != nil {
			// A thrown/rejected verify is treated as "not authenticated" (401),
			// not a 500: from the RS's perspective the token simply didn't
			// validate. The underlying reason is wrapped for logs/debugging.
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		id, ok := out.(*tokenIdentity)
		if !ok || id == nil {
			// verify returned null/undefined → unauthorized.
			return nil, fmt.Errorf("%w: verify returned no identity", auth.ErrInvalidToken)
		}
		return id.toTokenInfo(), nil
	}
}

// convertTokenIdentity runs ON the event loop (inside callJSHandler): it reads
// the verify() return value out of goja into a native *tokenIdentity, or
// returns (nil, nil) to signal "unauthorized" for a null/undefined result.
func convertTokenIdentity(vm *goja.Runtime, v goja.Value) (any, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, nil // unauthorized
	}
	obj := v.ToObject(vm)
	id := &tokenIdentity{}

	// subject / userId (either accepted; subject wins).
	if s := obj.Get("subject"); s != nil && !goja.IsUndefined(s) && !goja.IsNull(s) {
		id.subject = s.String()
	} else if s := obj.Get("userId"); s != nil && !goja.IsUndefined(s) && !goja.IsNull(s) {
		id.subject = s.String()
	}

	if sc := obj.Get("scopes"); sc != nil && !goja.IsUndefined(sc) && !goja.IsNull(sc) {
		if arr, ok := sc.Export().([]any); ok {
			for _, e := range arr {
				if s, ok := e.(string); ok {
					id.scopes = append(id.scopes, s)
				}
			}
		}
	}

	if ex := obj.Get("expiresAt"); ex != nil && !goja.IsUndefined(ex) && !goja.IsNull(ex) {
		switch t := ex.Export().(type) {
		case int64:
			id.expiresAt = time.Unix(t, 0)
			id.hasExpiry = true
		case float64:
			// Fractional Unix seconds.
			sec := int64(t)
			nsec := int64((t - float64(sec)) * 1e9)
			id.expiresAt = time.Unix(sec, nsec)
			id.hasExpiry = true
		case string:
			if parsed, perr := time.Parse(time.RFC3339, t); perr == nil {
				id.expiresAt = parsed
				id.hasExpiry = true
			}
			// An unparseable string leaves hasExpiry false → default TTL.
		}
	}

	if ev := obj.Get("extra"); ev != nil && !goja.IsUndefined(ev) && !goja.IsNull(ev) {
		if m, ok := ev.Export().(map[string]any); ok {
			id.extra = m
		}
	}

	return id, nil
}

// applyMCPAuth wraps the streamable handler with the bearer-token middleware and
// mounts the protected-resource-metadata endpoint on the same mux. metadataURL
// is the absolute URL of that endpoint on this server; it is echoed in the
// WWW-Authenticate header of every 401 so a client knows where to discover the
// authorization server(s).
func applyMCPAuth(mux *http.ServeMux, streamable http.Handler, path string, cfg *mcpAuthConfig, metadataURL string, verifier auth.TokenVerifier) {
	middleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: metadataURL,
		Scopes:              cfg.scopes,
	})
	mux.Handle(path, middleware(streamable))
	mux.Handle(mcpProtectedResourcePath, auth.ProtectedResourceMetadataHandler(cfg.meta))
}
