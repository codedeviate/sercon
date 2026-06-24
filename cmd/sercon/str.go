package main

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// strNamespace returns the `text.str.*` member map. Inputs are JS strings;
// outputs are JS strings unless noted. Mask arguments to trim/ltrim/rtrim
// follow PHP's "any character in this set" semantics. urlEncode uses
// form-style (`+` for space) to match recon's `urlencode`. Padding follows
// recon's str_pad shape: padChar defaults to a space, side is
// "right" (default) / "left" / "both".
func strNamespace(vm *goja.Runtime) map[string]any {
	requireString := func(label string, v goja.Value) string {
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			panic(vm.NewTypeError(label + ": expected a string"))
		}
		return v.String()
	}
	optString := func(v goja.Value, fallback string) string {
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			return fallback
		}
		return v.String()
	}

	tagsRE := regexp.MustCompile(`<[^>]*>`)
	brRE := regexp.MustCompile(`(?i)<br\s*/?>`)

	pad := func(s string, length int, padChar, side string) string {
		if padChar == "" {
			padChar = " "
		}
		runes := []rune(s)
		need := length - len(runes)
		if need <= 0 {
			return s
		}
		fill := strings.Repeat(padChar, need)
		// If the pad string is multi-char, trim to exactly `need` runes.
		fr := []rune(fill)
		if len(fr) > need {
			fill = string(fr[:need])
		}
		switch side {
		case "left":
			return fill + s
		case "both":
			left := need / 2
			right := need - left
			lfill := strings.Repeat(padChar, left)
			rfill := strings.Repeat(padChar, right)
			if lr := []rune(lfill); len(lr) > left {
				lfill = string(lr[:left])
			}
			if rr := []rune(rfill); len(rr) > right {
				rfill = string(rr[:right])
			}
			return lfill + s + rfill
		default: // "right" or anything unknown
			return s + fill
		}
	}

	return map[string]any{
		"trim": func(call goja.FunctionCall) goja.Value {
			s := requireString("trim", call.Argument(0))
			mask := optString(call.Argument(1), " \t\n\r\v\f")
			return vm.ToValue(strings.Trim(s, mask))
		},
		"ltrim": func(call goja.FunctionCall) goja.Value {
			s := requireString("ltrim", call.Argument(0))
			mask := optString(call.Argument(1), " \t\n\r\v\f")
			return vm.ToValue(strings.TrimLeft(s, mask))
		},
		"rtrim": func(call goja.FunctionCall) goja.Value {
			s := requireString("rtrim", call.Argument(0))
			mask := optString(call.Argument(1), " \t\n\r\v\f")
			return vm.ToValue(strings.TrimRight(s, mask))
		},
		"reverse": func(call goja.FunctionCall) goja.Value {
			s := requireString("reverse", call.Argument(0))
			r := []rune(s)
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
			return vm.ToValue(string(r))
		},
		"stripHtml": func(call goja.FunctionCall) goja.Value {
			s := requireString("stripHtml", call.Argument(0))
			return vm.ToValue(tagsRE.ReplaceAllString(s, ""))
		},
		"nl2br": func(call goja.FunctionCall) goja.Value {
			s := requireString("nl2br", call.Argument(0))
			tag := "<br>"
			if v := call.Argument(1); v != nil && !goja.IsUndefined(v) && v.ToBoolean() {
				tag = "<br/>"
			}
			s = strings.ReplaceAll(s, "\r\n", "\n")
			return vm.ToValue(strings.ReplaceAll(s, "\n", tag+"\n"))
		},
		"br2nl": func(call goja.FunctionCall) goja.Value {
			s := requireString("br2nl", call.Argument(0))
			return vm.ToValue(brRE.ReplaceAllString(s, "\n"))
		},
		"base64Encode": func(call goja.FunctionCall) goja.Value {
			s := requireString("base64Encode", call.Argument(0))
			return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(s)))
		},
		"base64Decode": func(call goja.FunctionCall) goja.Value {
			s := requireString("base64Decode", call.Argument(0))
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("base64Decode: %w", err)))
			}
			return vm.ToValue(string(b))
		},
		"base64UrlEncode": func(call goja.FunctionCall) goja.Value {
			s := requireString("base64UrlEncode", call.Argument(0))
			return vm.ToValue(base64.RawURLEncoding.EncodeToString([]byte(s)))
		},
		"base64UrlDecode": func(call goja.FunctionCall) goja.Value {
			s := requireString("base64UrlDecode", call.Argument(0))
			// Accept both padded (URLEncoding) and unpadded (RawURLEncoding)
			// URL-safe input so callers don't have to care which they got.
			b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("base64UrlDecode: %w", err)))
			}
			return vm.ToValue(string(b))
		},
		"urlEncode": func(call goja.FunctionCall) goja.Value {
			s := requireString("urlEncode", call.Argument(0))
			return vm.ToValue(url.QueryEscape(s))
		},
		"urlDecode": func(call goja.FunctionCall) goja.Value {
			s := requireString("urlDecode", call.Argument(0))
			d, err := url.QueryUnescape(s)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("urlDecode: %w", err)))
			}
			return vm.ToValue(d)
		},
		"htmlEntityDecode": func(call goja.FunctionCall) goja.Value {
			s := requireString("htmlEntityDecode", call.Argument(0))
			return vm.ToValue(html.UnescapeString(s))
		},
		"pad": func(call goja.FunctionCall) goja.Value {
			s := requireString("pad", call.Argument(0))
			length := int(call.Argument(1).ToInteger())
			padChar := optString(call.Argument(2), " ")
			side := optString(call.Argument(3), "right")
			return vm.ToValue(pad(s, length, padChar, side))
		},
		"lpad": func(call goja.FunctionCall) goja.Value {
			s := requireString("lpad", call.Argument(0))
			length := int(call.Argument(1).ToInteger())
			padChar := optString(call.Argument(2), " ")
			return vm.ToValue(pad(s, length, padChar, "left"))
		},
		"rpad": func(call goja.FunctionCall) goja.Value {
			s := requireString("rpad", call.Argument(0))
			length := int(call.Argument(1).ToInteger())
			padChar := optString(call.Argument(2), " ")
			return vm.ToValue(pad(s, length, padChar, "right"))
		},
		"sprintf": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(vm.NewTypeError("sprintf: format string required"))
			}
			format := call.Argument(0).String()
			args := make([]any, 0, len(call.Arguments)-1)
			for _, a := range call.Arguments[1:] {
				args = append(args, a.Export())
			}
			return vm.ToValue(fmt.Sprintf(format, args...))
		},
		"printf": func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(vm.NewTypeError("printf: format string required"))
			}
			format := call.Argument(0).String()
			args := make([]any, 0, len(call.Arguments)-1)
			for _, a := range call.Arguments[1:] {
				args = append(args, a.Export())
			}
			fmt.Printf(format, args...)
			return goja.Undefined()
		},
		"normalizeNewlines": func(call goja.FunctionCall) goja.Value {
			s := requireString("normalizeNewlines", call.Argument(0))
			style := optString(call.Argument(1), "lf")
			// Canonicalise to LF first, then convert.
			s = strings.ReplaceAll(s, "\r\n", "\n")
			s = strings.ReplaceAll(s, "\r", "\n")
			switch style {
			case "crlf":
				return vm.ToValue(strings.ReplaceAll(s, "\n", "\r\n"))
			case "cr":
				return vm.ToValue(strings.ReplaceAll(s, "\n", "\r"))
			case "lf":
				return vm.ToValue(s)
			default:
				panic(vm.NewTypeError("normalizeNewlines: style must be 'lf', 'crlf', or 'cr'"))
			}
		},
	}
}
