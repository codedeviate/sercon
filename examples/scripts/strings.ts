// Demonstrates api.text.str.* — trimming, padding, encoding round-trips, and
// printf-style formatting. JSON.stringify is used where literal whitespace
// would otherwise be invisible in the output.

api.runtime.log("=== api.text.str.* ===");

// Trimming with a custom mask (any character in the mask is stripped).
api.runtime.log("trim '/':   ", JSON.stringify(api.text.str.trim("///hello///", "/")));

// Padding (left = pad on the left, both = centre-ish).
api.runtime.log("lpad zero:  ", api.text.str.lpad("7", 4, "0"));
api.runtime.log("rpad dots:  ", api.text.str.rpad("ab", 6, "."));
api.runtime.log("pad both:   ", api.text.str.pad("ab", 6, ".", "both"));

// Rune-aware string reverse (not byte-reverse).
api.runtime.log("reverse:    ", api.text.str.reverse("café"));

// HTML
api.runtime.log("stripHtml:  ", api.text.str.stripHtml("<p>hi <b>there</b></p>"));
api.runtime.log("nl2br:      ", JSON.stringify(api.text.str.nl2br("a\nb")));
api.runtime.log("htmlEntity: ", api.text.str.htmlEntityDecode("&lt;p&gt;&amp;"));

// Base64 / URL round-trips.
api.runtime.log("b64 encode: ", api.text.str.base64Encode("hello"));
api.runtime.log("b64 decode: ", api.text.str.base64Decode("aGVsbG8="));
api.runtime.log("url encode: ", api.text.str.urlEncode("a b/c"));
api.runtime.log("url decode: ", api.text.str.urlDecode("a+b%2Fc"));

// sprintf uses Go fmt verbs (%s, %d, %x, %.2f, %v, %t, %q, ...).
api.runtime.log("sprintf:    ", api.text.str.sprintf("%-8s %d (%.2f)", "answer", 42, 3.14159));

// Line-ending canonicalisation.
api.runtime.log("normalize:  ", JSON.stringify(api.text.str.normalizeNewlines("a\r\nb\rc", "lf")));
