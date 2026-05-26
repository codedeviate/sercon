// Demonstrates api.str.* — trimming, padding, encoding round-trips, and
// printf-style formatting. JSON.stringify is used where literal whitespace
// would otherwise be invisible in the output.

api.log("=== api.str.* ===");

// Trimming with a custom mask (any character in the mask is stripped).
api.log("trim '/':   ", JSON.stringify(api.str.trim("///hello///", "/")));

// Padding (left = pad on the left, both = centre-ish).
api.log("lpad zero:  ", api.str.lpad("7", 4, "0"));
api.log("rpad dots:  ", api.str.rpad("ab", 6, "."));
api.log("pad both:   ", api.str.pad("ab", 6, ".", "both"));

// Rune-aware string reverse (not byte-reverse).
api.log("reverse:    ", api.str.reverse("café"));

// HTML
api.log("stripHtml:  ", api.str.stripHtml("<p>hi <b>there</b></p>"));
api.log("nl2br:      ", JSON.stringify(api.str.nl2br("a\nb")));
api.log("htmlEntity: ", api.str.htmlEntityDecode("&lt;p&gt;&amp;"));

// Base64 / URL round-trips.
api.log("b64 encode: ", api.str.base64Encode("hello"));
api.log("b64 decode: ", api.str.base64Decode("aGVsbG8="));
api.log("url encode: ", api.str.urlEncode("a b/c"));
api.log("url decode: ", api.str.urlDecode("a+b%2Fc"));

// sprintf uses Go fmt verbs (%s, %d, %x, %.2f, %v, %t, %q, ...).
api.log("sprintf:    ", api.str.sprintf("%-8s %d (%.2f)", "answer", 42, 3.14159));

// Line-ending canonicalisation.
api.log("normalize:  ", JSON.stringify(api.str.normalizeNewlines("a\r\nb\rc", "lf")));
