package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash/crc32"

	"github.com/dop251/goja"
	"golang.org/x/crypto/sha3"
	"lukechampine.com/blake3"
)

// hashNamespace builds the `crypto.hash.*` member map. Inputs are interpreted as
// UTF-8 byte sequences; outputs are lowercase hex strings. crc32 uses the
// IEEE polynomial (the universal default).
func hashNamespace(vm *goja.Runtime) map[string]any {
	stringInput := func(call goja.FunctionCall) []byte {
		arg := call.Argument(0)
		if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
			panic(vm.NewTypeError("hash: argument 1 must be a string"))
		}
		return []byte(arg.String())
	}

	return map[string]any{
		"md5": func(call goja.FunctionCall) goja.Value {
			sum := md5.Sum(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"sha1": func(call goja.FunctionCall) goja.Value {
			sum := sha1.Sum(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"sha256": func(call goja.FunctionCall) goja.Value {
			sum := sha256.Sum256(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"sha384": func(call goja.FunctionCall) goja.Value {
			sum := sha512.Sum384(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"sha512": func(call goja.FunctionCall) goja.Value {
			sum := sha512.Sum512(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"sha3_256": func(call goja.FunctionCall) goja.Value {
			sum := sha3.Sum256(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"sha3_512": func(call goja.FunctionCall) goja.Value {
			sum := sha3.Sum512(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"blake3": func(call goja.FunctionCall) goja.Value {
			sum := blake3.Sum256(stringInput(call))
			return vm.ToValue(hex.EncodeToString(sum[:]))
		},
		"crc32": func(call goja.FunctionCall) goja.Value {
			sum := crc32.ChecksumIEEE(stringInput(call))
			return vm.ToValue(fmt.Sprintf("%08x", sum))
		},
	}
}
