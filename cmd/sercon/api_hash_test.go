package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestHashNamespace verifies the api.hash.* surface against well-known
// vectors for the empty string and the canonical "abc" input. Tests live
// inside the package so they can drive hashNamespace through a real Engine.
func TestHashNamespace(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("hash", func(vm *goja.Runtime, _ *eventloop.EventLoop) map[string]any {
		return hashNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}

	// Known test vectors. Lowercase hex throughout.
	script := `
function eq(label, got, want) {
  if (got !== want) throw new Error(label + ": got " + got + ", want " + want);
}

// Empty-string vectors
eq("md5('')",       hash.md5(""),       "d41d8cd98f00b204e9800998ecf8427e");
eq("sha1('')",      hash.sha1(""),      "da39a3ee5e6b4b0d3255bfef95601890afd80709");
eq("sha256('')",    hash.sha256(""),    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
eq("sha384('')",    hash.sha384(""),    "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b");
eq("sha512('')",    hash.sha512(""),    "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e");
eq("sha3_256('')",  hash.sha3_256(""),  "a7ffc6f8bf1ed76651c14756a061d662f580ff4de43b49fa82d80a4b80f8434a");
eq("sha3_512('')",  hash.sha3_512(""),  "a69f73cca23a9ac5c8b567dc185a756e97c982164fe25859e0d1dcc1475c80a615b2123af1f5f94c11e3e9402c3ac558f500199d95b6d3e301758586281dcd26");
eq("blake3('')",    hash.blake3(""),    "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262");
eq("crc32('')",     hash.crc32(""),     "00000000");

// 'abc' vectors
eq("md5('abc')",      hash.md5("abc"),      "900150983cd24fb0d6963f7d28e17f72");
eq("sha1('abc')",     hash.sha1("abc"),     "a9993e364706816aba3e25717850c26c9cd0d89d");
eq("sha256('abc')",   hash.sha256("abc"),   "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
eq("sha384('abc')",   hash.sha384("abc"),   "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7");
eq("sha512('abc')",   hash.sha512("abc"),   "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f");
eq("sha3_256('abc')", hash.sha3_256("abc"), "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532");
eq("sha3_512('abc')", hash.sha3_512("abc"), "b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0");
eq("blake3('abc')",   hash.blake3("abc"),   "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85");
eq("crc32('abc')",    hash.crc32("abc"),    "352441c2");
`
	if _, err := eng.Run(context.Background(), "hash_test.ts", script); err != nil {
		t.Fatalf("hash test script: %v", err)
	}
}
