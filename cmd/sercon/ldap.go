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

// ldapNamespace wires `db.ldap.*`. `open(url)` dials an LDAP server
// (`ldap://host:port` or `ldaps://...`) and does an anonymous bind,
// then returns a stateful handle `{ rootDSE, search, close }`. A
// directory-inspection binding — anonymous read queries, not a
// write / modify surface.
//
// Library: github.com/go-ldap/ldap/v3 (the well-maintained pure-Go
// client).
func ldapNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, ldapOpenExtract,
			func(ctx context.Context, args ldapOpenArgs) (map[string]any, error) {
				return ldapOpen(vm, loop, args)
			}),
	}
}

// ldapOpenArgs carries the on-loop-extracted arguments of db.ldap.open.
type ldapOpenArgs struct {
	url      string
	bindDN   string
	password string
}

func ldapOpenExtract(call goja.FunctionCall) (ldapOpenArgs, error) {
	url := call.Argument(0).String()
	if url == "" {
		return ldapOpenArgs{}, errors.New("ldap.open: url required (e.g. ldap://localhost:389)")
	}
	// Anonymous bind — optional bindDN / password via opts.
	opts := optAt(call, 1)
	return ldapOpenArgs{
		url:      url,
		bindDN:   optString(opts, "bindDN", ""),
		password: optString(opts, "password", ""),
	}, nil
}

// ldapSearchArgs carries the on-loop-extracted arguments of the handle's
// search(baseDN, filter, attrs?) member.
type ldapSearchArgs struct {
	baseDN string
	filter string
	attrs  []string
}

func ldapSearchExtract(call goja.FunctionCall) (ldapSearchArgs, error) {
	filter := call.Argument(1).String()
	if filter == "" {
		filter = "(objectClass=*)"
	}
	attrs := []string{}
	if pa, err := pathsArg(call.Argument(2), "ldap.search.attrs"); err == nil {
		attrs = pa
	}
	return ldapSearchArgs{
		baseDN: call.Argument(0).String(),
		filter: filter,
		attrs:  attrs,
	}, nil
}

func ldapOpen(vm *goja.Runtime, loop *eventloop.EventLoop, args ldapOpenArgs) (map[string]any, error) {
	conn, err := ldap.DialURL(args.url)
	if err != nil {
		return nil, fmt.Errorf("ldap.open: dial: %w", err)
	}
	if args.bindDN != "" {
		if err := conn.Bind(args.bindDN, args.password); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("ldap.open: bind: %w", err)
		}
	}

	return map[string]any{
		// rootDSE() reads the server's Root DSE — the anonymous
		// metadata entry that advertises naming contexts, supported
		// controls, vendor, etc.
		"rootDSE": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(_ context.Context, _ struct{}) (*scriptengine.Ordered, error) {
				req := ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
					0, 0, false, "(objectClass=*)", []string{"*", "+"}, nil)
				res, err := conn.Search(req)
				if err != nil {
					return nil, fmt.Errorf("ldap.rootDSE: %w", err)
				}
				if len(res.Entries) == 0 {
					return scriptengine.NewOrdered(), nil
				}
				return ldapEntryToMap(res.Entries[0]), nil
			}).Func,
		// search(baseDN, filter, attrs?) — a generic subtree search.
		"search": scriptengine.PromisifyAsync(vm, loop, ldapSearchExtract,
			func(_ context.Context, args ldapSearchArgs) ([]*scriptengine.Ordered, error) {
				req := ldap.NewSearchRequest(args.baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
					0, 0, false, args.filter, args.attrs, nil)
				res, err := conn.Search(req)
				if err != nil {
					return nil, fmt.Errorf("ldap.search: %w", err)
				}
				out := make([]*scriptengine.Ordered, 0, len(res.Entries))
				for _, e := range res.Entries {
					out = append(out, ldapEntryToMap(e))
				}
				return out, nil
			}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(_ context.Context, _ struct{}) (any, error) {
				if err := conn.Close(); err != nil {
					return nil, fmt.Errorf("ldap.close: %w", err)
				}
				return nil, nil
			}).Func,
	}, nil
}

// ldapEntryToMap flattens an LDAP entry into `{ dn, <attr>: [values] }`.
// Multi-valued attributes (the common case in LDAP) stay as arrays so
// the shape is uniform regardless of cardinality. Keys emit in stable
// order: `dn` first, then each attribute in e.Attributes order, so
// JSON.stringify output is reproducible for canonical-hash callers.
func ldapEntryToMap(e *ldap.Entry) *scriptengine.Ordered {
	out := scriptengine.NewOrdered().Set("dn", e.DN)
	for _, attr := range e.Attributes {
		vals := make([]any, len(attr.Values))
		for i, v := range attr.Values {
			vals[i] = v
		}
		out.Set(attr.Name, vals)
	}
	return out
}
