package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func textDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"str.trim":              {Summary: "Strip whitespace (or any char in the optional mask string) from both ends."},
		"str.ltrim":             {Summary: "Like trim, left side only."},
		"str.rtrim":             {Summary: "Like trim, right side only."},
		"str.reverse":           {Summary: "Rune-aware reversal — `reverse('café')` is `'éfac'`."},
		"str.stripHtml":         {Summary: "Remove HTML tags and decode common entities."},
		"str.nl2br":             {Summary: "Replace newlines with <br> (or <br/> when xhtml=true)."},
		"str.br2nl":             {Summary: "Inverse of nl2br: <br>, <br/>, <br /> → '\\n'."},
		"str.base64Encode":      {Summary: "Standard base64 (with padding)."},
		"str.base64Decode":      {Summary: "Standard base64; URL-safe input is accepted via auto-detect."},
		"str.urlEncode":         {Summary: "Form-encoding ('+' for space). For path segments use encodeURIComponent (provided by goja)."},
		"str.urlDecode":         {Summary: "Inverse of urlEncode."},
		"str.htmlEntityDecode":  {Summary: "Decode named and numeric HTML entities to their UTF-8 equivalents."},
		"str.pad":               {Summary: "Pad to `len` with `padChar` (default ' '). `side` is 'right' (default), 'left', or 'both'."},
		"str.lpad":              {Summary: "Shortcut for pad(side: 'left')."},
		"str.rpad":              {Summary: "Shortcut for pad(side: 'right')."},
		"str.sprintf":           {Summary: "Go's fmt verbs (%s, %d, %x, %.2f, %v, %t, %q, …) — not PHP's."},
		"str.printf":            {Summary: "sprintf + write to stdout."},
		"str.normalizeNewlines": {Summary: "Canonicalise any mix of \\r\\n, \\r, \\n to the requested style ('lf' | 'crlf' | 'cr')."},
		"charset.detect":        {Summary: "Detect the most-likely charset of a byte sequence (saintfish/chardet). Returns top guess + candidates."},
		"charset.decode":        {Summary: "Decode bytes in a named charset to a UTF-8 string."},
		"charset.encode":        {Summary: "Encode a UTF-8 string to bytes in the named charset."},
		"preg.match":            {Summary: "First hit of /pattern/flags against subject, or null. Returns { match, groups, index }; optional groups that didn't match surface as empty strings."},
		"preg.matchAll":         {Summary: "Every hit of /pattern/flags against subject, as an array of { match, groups, index } objects."},
		"preg.replace":          {Summary: "Substitute every match of /pattern/flags in subject. Replacement uses Go's $1 / ${1} backref syntax — PHP's \\1 form is NOT translated."},
		"preg2.match":           {Summary: "First hit of /pattern/flags via regexp2 (PCRE). Supports lookahead/lookbehind/backreferences. Same { match, groups, index } shape as preg. No linear-time guarantee."},
		"preg2.matchAll":        {Summary: "Every hit of /pattern/flags via regexp2 (PCRE), as an array of { match, groups, index }."},
		"preg2.replace":         {Summary: "Substitute every match of /pattern/flags via regexp2. Replacement uses .NET $1 / ${1} syntax. Backtracking engine — keep a timeout around untrusted input."},
		"jq.query":              {Summary: "Run a jq filter over data and return the first emitted value (or null)."},
		"jq.queryAll":           {Summary: "Run a jq filter and drain the iterator into an array."},
		"diff.compare":          {Summary: "Unified-diff two text inputs. opts: context (default 3), fromFile / toFile (default 'a' / 'b'). Binary inputs return { binary: true } with an empty diff."},
	}
}
