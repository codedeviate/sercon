package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func ldapEng(t *testing.T) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := eng.RegisterNamespaceFactory("ldap", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return ldapNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	return eng
}

func TestLDAP_EmptyURLThrows(t *testing.T) {
	if _, err := ldapEng(t).Run(context.Background(), "x.ts", `await ldap.open("");`); err == nil {
		t.Error("empty url should throw")
	}
}

func TestLDAP_BadURLThrows(t *testing.T) {
	if _, err := ldapEng(t).Run(context.Background(), "x.ts", `await ldap.open("not a url");`); err == nil {
		t.Error("bad url should throw")
	}
}

func TestLDAP_ConnectionRefusedThrows(t *testing.T) {
	_, err := ldapEng(t).Run(context.Background(), "x.ts", `await ldap.open("ldap://127.0.0.1:1");`)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "ldap.open") {
		t.Errorf("expected ldap.open prefix; got %v", err)
	}
}
