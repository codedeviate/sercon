package main

import (
	"reflect"
	"strings"
	"testing"
)

// fixture is the same map shape goja would hand us if a script passed in
// `JSON.parse(...)` of a typical record set.
var fixture = map[string]any{
	"users": []any{
		map[string]any{"name": "alice", "age": 30, "admin": true},
		map[string]any{"name": "bob", "age": 25, "admin": false},
		map[string]any{"name": "carol", "age": 35, "admin": true},
	},
	"meta": map[string]any{"count": 3},
}

// runJqQuery covers a handful of jq idioms: field access, indexing,
// `.[]` exploder, `select`, and `add`. Each entry pins the expected
// result type and value.
func TestJq_RunJqQuery(t *testing.T) {
	cases := []struct {
		name, filter string
		want         any
		count        int // expected number of results from queryAll
	}{
		{
			name:   "scalar field",
			filter: ".meta.count",
			want:   3,
			count:  1,
		},
		{
			name:   "first array element",
			filter: ".users[0].name",
			want:   "alice",
			count:  1,
		},
		{
			name:   "exploded names",
			filter: ".users[].name",
			want:   "alice", // first
			count:  3,
		},
		{
			name:   "filtered admins",
			filter: ".users[] | select(.admin) | .name",
			want:   "alice",
			count:  2,
		},
		{
			name:   "computed sum",
			filter: "[.users[].age] | add",
			want:   90,
			count:  1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			first, err := runJqQuery(fixture, c.filter, 1)
			if err != nil {
				t.Fatalf("first: %v", err)
			}
			if len(first) != 1 {
				t.Fatalf("expected 1 result with limit=1, got %d", len(first))
			}
			if !reflect.DeepEqual(first[0], c.want) {
				t.Errorf("first[0]: got %v (%T), want %v (%T)", first[0], first[0], c.want, c.want)
			}
			all, err := runJqQuery(fixture, c.filter, 0)
			if err != nil {
				t.Fatalf("all: %v", err)
			}
			if len(all) != c.count {
				t.Errorf("queryAll count: got %d, want %d", len(all), c.count)
			}
		})
	}
}

// A syntactically invalid filter must surface a parse error rather than
// silently returning empty results.
func TestJq_ParseError(t *testing.T) {
	_, err := runJqQuery(fixture, "this is not valid jq", 0)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

// A runtime error from inside the iterator (e.g. type mismatch) must also
// surface as a Go error — gojq emits these as in-band values so we have
// to type-assert them out.
func TestJq_RuntimeError(t *testing.T) {
	// `.users + 1` is a type error: array + number isn't defined in jq.
	_, err := runJqQuery(fixture, ".users + 1", 0)
	if err == nil {
		t.Fatal("expected runtime error from gojq")
	}
}

// Missing fields with the optional-access operator return nil — we want
// that to surface as JS null via the goja.Export -> Go nil path, not as
// an error.
func TestJq_OptionalMissing(t *testing.T) {
	results, err := runJqQuery(fixture, ".does.not.exist?", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0] != nil {
		t.Errorf("expected one nil result, got %v", results)
	}
}
