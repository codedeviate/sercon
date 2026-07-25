package main

import (
	"context"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func TestTextCaseBinding(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := registerSurface(eng); err != nil {
		t.Fatalf("register: %v", err)
	}
	src := `
		runtime.assert.equal(text.case.snake("myVarName"), "my_var_name");
		runtime.assert.equal(text.case.kebab("userID"), "user-id");
		runtime.assert.equal(text.case.pascal("http-server"), "HttpServer");
		runtime.assert.equal(text.case.convert("myVar", "screamingSnake"), "MY_VAR");
		runtime.assert.equal(JSON.stringify(text.case.split("fooBarBaz")), JSON.stringify(["foo","bar","baz"]));
		runtime.assert.equal(text.case.detect("a_b"), "snake");
		runtime.assert.equal(text.case.header("aB"), text.case.train("aB")); // alias
		runtime.assert.ok(text.case.names().length === 16);
		let threw = false;
		try { text.case.convert("x", "nope"); } catch (e) { threw = true; }
		runtime.assert.ok(threw, "unknown case name should throw");
	`
	if _, err := eng.Run(context.Background(), "case.ts", src); err != nil {
		t.Fatalf("script: %v", err)
	}
}
