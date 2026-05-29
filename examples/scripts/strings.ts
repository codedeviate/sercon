// Demonstrates text.str.* — trimming, padding, encoding round-trips, and
// printf-style formatting. JSON.stringify is used where literal whitespace
// would otherwise be invisible in the output.

runtime.log("=== text.str.* ===");

// Trimming with a custom mask (any character in the mask is stripped).
runtime.log("trim '/':   ", JSON.stringify(text.str.trim("///hello///", "/")));

// Padding (left = pad on the left, both = centre-ish).
runtime.log("lpad zero:  ", text.str.lpad("7", 4, "0"));
runtime.log("rpad dots:  ", text.str.rpad("ab", 6, "."));
runtime.log("pad both:   ", text.str.pad("ab", 6, ".", "both"));

// Rune-aware string reverse (not byte-reverse).
runtime.log("reverse:    ", text.str.reverse("café"));

// HTML
runtime.log("stripHtml:  ", text.str.stripHtml("<p>hi <b>there</b></p>"));
runtime.log("nl2br:      ", JSON.stringify(text.str.nl2br("a\nb")));
runtime.log("htmlEntity: ", text.str.htmlEntityDecode("&lt;p&gt;&amp;"));

// Base64 / URL round-trips.
runtime.log("b64 encode: ", text.str.base64Encode("hello"));
runtime.log("b64 decode: ", text.str.base64Decode("aGVsbG8="));
runtime.log("url encode: ", text.str.urlEncode("a b/c"));
runtime.log("url decode: ", text.str.urlDecode("a+b%2Fc"));

// sprintf uses Go fmt verbs (%s, %d, %x, %.2f, %v, %t, %q, ...).
runtime.log("sprintf:    ", text.str.sprintf("%-8s %d (%.2f)", "answer", 42, 3.14159));

// Line-ending canonicalisation.
runtime.log("normalize:  ", JSON.stringify(text.str.normalizeNewlines("a\r\nb\rc", "lf")));
