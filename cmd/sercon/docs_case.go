package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// caseDocs documents every text.case.* member. Merged into textDocs().
func caseDocs() map[string]scriptengine.MemberDoc {
	strParam := []scriptengine.Param{{Name: "input", Type: "string", Desc: "The string to convert. Word boundaries are auto-detected (camelCase, snake_case, kebab-case, spaces, dot/path, and acronym runs like HTTPServer)."}}
	conv := func(summary, ret, example string) scriptengine.MemberDoc {
		return scriptengine.MemberDoc{
			Summary:    summary,
			Params:     strParam,
			ReturnType: "string",
			Returns:    ret,
			Errors:     "Throws a TypeError if input is missing, null, or undefined.",
			Example:    example,
		}
	}
	return map[string]scriptengine.MemberDoc{
		"case.camel":          conv("Convert to camelCase (first word lower, rest capitalized, no separator).", "string — e.g. \"myVarName\".", `text.case.camel("my-var-name"); // "myVarName"`),
		"case.pascal":         conv("Convert to PascalCase (every word capitalized, no separator).", "string — e.g. \"MyVarName\".", `text.case.pascal("my_var"); // "MyVar"`),
		"case.snake":          conv("Convert to snake_case (all lower, underscore-separated).", "string — e.g. \"my_var_name\".", `text.case.snake("myVarName"); // "my_var_name"`),
		"case.screamingSnake": conv("Convert to SCREAMING_SNAKE_CASE (all upper, underscore-separated).", "string — e.g. \"MY_VAR_NAME\".", `text.case.screamingSnake("myVar"); // "MY_VAR"`),
		"case.ada":            conv("Convert to Ada_Case (every word capitalized, underscore-separated).", "string — e.g. \"My_Var_Name\".", `text.case.ada("my-var"); // "My_Var"`),
		"case.camelSnake":     conv("Convert to camel_Snake_Case (first word lower, rest capitalized, underscore-separated).", "string — e.g. \"my_Var_Name\".", `text.case.camelSnake("my-var"); // "my_Var"`),
		"case.kebab":          conv("Convert to kebab-case (all lower, hyphen-separated).", "string — e.g. \"my-var-name\".", `text.case.kebab("myVarName"); // "my-var-name"`),
		"case.train":          conv("Convert to Train-Case (every word capitalized, hyphen-separated).", "string — e.g. \"My-Var-Name\".", `text.case.train("my_var"); // "My-Var"`),
		"case.screamingKebab": conv("Convert to SCREAMING-KEBAB-CASE (all upper, hyphen-separated).", "string — e.g. \"MY-VAR-NAME\".", `text.case.screamingKebab("myVar"); // "MY-VAR"`),
		"case.flat":           conv("Convert to flatcase (all lower, no separator). Lossy: boundaries are gone and cannot be recovered.", "string — e.g. \"myvarname\".", `text.case.flat("myVarName"); // "myvarname"`),
		"case.upperFlat":      conv("Convert to UPPERFLATCASE (all upper, no separator). Lossy: boundaries are gone.", "string — e.g. \"MYVARNAME\".", `text.case.upperFlat("myVar"); // "MYVAR"`),
		"case.dot":            conv("Convert to dot.case (all lower, dot-separated).", "string — e.g. \"my.var.name\".", `text.case.dot("myVarName"); // "my.var.name"`),
		"case.path":           conv("Convert to path/case (all lower, slash-separated).", "string — e.g. \"my/var/name\".", `text.case.path("myVarName"); // "my/var/name"`),
		"case.title":          conv("Convert to Title Case (every word capitalized, space-separated). Synonym of capital.", "string — e.g. \"My Var Name\".", `text.case.title("my_var_name"); // "My Var Name"`),
		"case.sentence":       conv("Convert to Sentence case (first word capitalized, rest lower, space-separated).", "string — e.g. \"My var name\".", `text.case.sentence("my_var_name"); // "My var name"`),
		"case.capital":        conv("Convert to Capital Case (every word capitalized, space-separated). Synonym of title.", "string — e.g. \"My Var Name\".", `text.case.capital("my_var_name"); // "My Var Name"`),

		"case.split": {
			Summary:    "Tokenize any input into an array of lowercased words (the primitive every converter builds on).",
			Params:     strParam,
			ReturnType: "string[]",
			Returns:    "string[] — lowercased words; boundaries detected at lower→upper transitions, acronym→word (HTTPServer→[http,server]), and separators (_ - . / whitespace). Empty/separator-only input → [].",
			Errors:     "Throws a TypeError if input is missing, null, or undefined.",
			Example:    `text.case.split("getHTTPCode"); // ["get","http","code"]`,
		},
		"case.detect": {
			Summary:    "Best-effort guess of the input's case name (heuristic).",
			Params:     strParam,
			ReturnType: "string",
			Returns:    "string — the first case name whose converter reproduces the input exactly, or \"unknown\". Multiword inputs detect cleanly; a single lowercase word resolves to \"camel\"; empty input is \"unknown\".",
			Errors:     "Throws a TypeError if input is missing, null, or undefined.",
			Example:    `text.case.detect("my_var"); // "snake"`,
		},
		"case.convert": {
			Summary:    "Convert input to the named case (dynamic dispatch; accepts canonical names and aliases).",
			Params:     []scriptengine.Param{{Name: "input", Type: "string", Desc: "The string to convert."}, {Name: "name", Type: "string", Desc: "A case name from names() (e.g. \"snake\", \"kebab\") or an alias (header/cobol/slug)."}},
			ReturnType: "string",
			Returns:    "string — input rendered in the requested case.",
			Errors:     "Throws a TypeError if input/name is missing; throws if name is not a known case (the message lists valid names).",
			Example:    `text.case.convert("userID", "kebab"); // "user-id"`,
		},
		"case.names": {
			Summary:    "List the canonical case names (drives convert() and detect(); excludes aliases).",
			Params:     []scriptengine.Param{},
			ReturnType: "string[]",
			Returns:    "string[] — the 16 canonical case names in priority order.",
			Errors:     "None.",
			Example:    `text.case.names(); // ["camel","pascal","snake",...]`,
		},

		"case.header": conv("Alias of train (Header-Case, e.g. Content-Type).", "string — e.g. \"My-Var-Name\".", `text.case.header("content-type"); // "Content-Type"`),
		"case.cobol":  conv("Alias of screamingKebab (COBOL-CASE).", "string — e.g. \"MY-VAR-NAME\".", `text.case.cobol("myVar"); // "MY-VAR"`),
		"case.slug":   conv("Alias of kebab. NOTE: kebab only — does NOT transliterate or strip diacritics/punctuation.", "string — e.g. \"my-var-name\".", `text.case.slug("My Var"); // "my-var"`),
	}
}
