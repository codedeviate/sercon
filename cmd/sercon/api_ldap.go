package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/go-ldap/ldap/v3"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// ldapNamespace wires `api.db.ldap.*`. `open(url)` dials an LDAP server
// (`ldap://host:port` or `ldaps://...`) and does an anonymous bind,
// then returns a stateful handle `{ rootDSE, search, close }`. A
// directory-inspection binding — anonymous read queries, not a
// write / modify surface.
//
// Library: github.com/go-ldap/ldap/v3 (the well-maintained pure-Go
// client).
func ldapNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
			return ldapOpen(vm, loop, call)
		}),
	}
}

func ldapOpen(vm *goja.Runtime, loop *eventloop.EventLoop, call goja.FunctionCall) (map[string]any, error) {
	url := call.Argument(0).String()
	if url == "" {
		return nil, errors.New("ldap.open: url required (e.g. ldap://localhost:389)")
	}
	conn, err := ldap.DialURL(url)
	if err != nil {
		return nil, fmt.Errorf("ldap.open: dial: %w", err)
	}
	// Anonymous bind — optional bindDN / password via opts.
	opts := optAt(call, 1)
	bindDN := optString(opts, "bindDN", "")
	password := optString(opts, "password", "")
	if bindDN != "" {
		if err := conn.Bind(bindDN, password); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("ldap.open: bind: %w", err)
		}
	}

	return map[string]any{
		// rootDSE() reads the server's Root DSE — the anonymous
		// metadata entry that advertises naming contexts, supported
		// controls, vendor, etc.
		"rootDSE": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
			req := ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
				0, 0, false, "(objectClass=*)", []string{"*", "+"}, nil)
			res, err := conn.Search(req)
			if err != nil {
				return nil, fmt.Errorf("ldap.rootDSE: %w", err)
			}
			if len(res.Entries) == 0 {
				return map[string]any{}, nil
			}
			return ldapEntryToMap(res.Entries[0]), nil
		}).Func,
		// search(baseDN, filter, attrs?) — a generic subtree search.
		"search": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) ([]map[string]any, error) {
			baseDN := call.Argument(0).String()
			filter := call.Argument(1).String()
			if filter == "" {
				filter = "(objectClass=*)"
			}
			attrs := []string{}
			if pa, err := pathsArg(call.Argument(2), "ldap.search.attrs"); err == nil {
				attrs = pa
			}
			req := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
				0, 0, false, filter, attrs, nil)
			res, err := conn.Search(req)
			if err != nil {
				return nil, fmt.Errorf("ldap.search: %w", err)
			}
			out := make([]map[string]any, 0, len(res.Entries))
			for _, e := range res.Entries {
				out = append(out, ldapEntryToMap(e))
			}
			return out, nil
		}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			if err := conn.Close(); err != nil {
				return nil, fmt.Errorf("ldap.close: %w", err)
			}
			return nil, nil
		}).Func,
	}, nil
}

// ldapEntryToMap flattens an LDAP entry into `{ dn, <attr>: [values] }`.
// Multi-valued attributes (the common case in LDAP) stay as arrays so
// the shape is uniform regardless of cardinality.
func ldapEntryToMap(e *ldap.Entry) map[string]any {
	out := map[string]any{"dn": e.DN}
	for _, attr := range e.Attributes {
		out[attr.Name] = attr.Values
	}
	return out
}
