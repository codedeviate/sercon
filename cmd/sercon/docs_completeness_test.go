package main

import (
	"fmt"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// docsByNamespace maps every reserved-global namespace (+ console) to its
// structured-docs function. Keep this in sync with registerSurface's
// SetMemberDocsStructured calls in main.go.
func docsByNamespace() map[string]map[string]scriptengine.MemberDoc {
	return map[string]map[string]scriptengine.MemberDoc{
		"runtime":  runtimeDocs(),
		"crypto":   cryptoDocs(),
		"text":     textDocs(),
		"codec":    codecDocs(),
		"fs":       fsDocs(),
		"net":      netDocs(),
		"db":       dbDocs(),
		"services": servicesDocs(),
		"tui":      tuiDocs(),
		"image":    imageDocs(),
		"web":      webDocs(),
		"server":   serverDocs(),
		"console":  consoleDocs(),
	}
}

// sweptNamespaces lists the namespaces whose MemberDocs have been brought to the
// completeness standard. Each Part-B task appends its namespace here as its final
// step; the final task asserts this set covers every namespace in
// docsByNamespace() (see TestDocsComplete_CoversAllNamespaces, added later).
var sweptNamespaces = []string{"runtime", "crypto", "text", "codec", "fs", "net", "db", "services"}

// checkMember asserts a single MemberDoc meets the completeness standard:
// non-empty Summary/ReturnType/Returns/Errors/Example, and every Param has
// non-empty Name/Type/Desc.
func checkMember(t *testing.T, ns, key string, d scriptengine.MemberDoc) {
	t.Helper()
	where := fmt.Sprintf("%s.%s", ns, key)
	if d.Summary == "" {
		t.Errorf("%s: empty Summary", where)
	}
	if d.ReturnType == "" {
		t.Errorf("%s: empty ReturnType", where)
	}
	if d.Returns == "" {
		t.Errorf("%s: empty Returns", where)
	}
	if d.Errors == "" {
		t.Errorf("%s: empty Errors", where)
	}
	if d.Example == "" {
		t.Errorf("%s: empty Example", where)
	}
	for i, p := range d.Params {
		if p.Name == "" || p.Type == "" || p.Desc == "" {
			t.Errorf("%s: param[%d] missing Name/Type/Desc (%+v)", where, i, p)
		}
	}
}

func TestDocsComplete(t *testing.T) {
	all := docsByNamespace()
	for _, ns := range sweptNamespaces {
		docs, ok := all[ns]
		if !ok {
			t.Fatalf("sweptNamespaces names unknown namespace %q", ns)
		}
		for key, d := range docs {
			checkMember(t, ns, key, d)
		}
	}
}
