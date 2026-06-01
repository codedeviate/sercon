package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func codecDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"compression.algos": {
			Summary:    "Available compression algorithm names (gzip / deflate / zlib / bzip2 / zstd / brotli / lz4 / xz / snappy).",
			ReturnType: "string[]",
			Returns:    "string[] — the nine supported algorithm names, lowercase, in registration order.",
			Errors:     "Never throws.",
			Example:    `const algos = codec.compression.algos(); // ["gzip", "deflate", ...]`,
		},
		"compression.compress": {
			Summary: "Compress data with the named algorithm. Returns Uint8Array. Async.",
			Params: []scriptengine.Param{
				{Name: "algo", Type: "string", Desc: "Algorithm name (case-insensitive): gzip / deflate / zlib / bzip2 / zstd / brotli / lz4 / xz / snappy."},
				{Name: "data", Type: "string | Uint8Array | ArrayBuffer", Desc: "Input bytes. Strings are interpreted as their UTF-8 byte sequence."},
			},
			ReturnType: "Promise<Uint8Array>",
			Returns:    "Promise<Uint8Array> — the compressed bytes.",
			Errors:     "Throws if data is undefined/null or an unsupported type, the algorithm name is unknown, or the underlying compressor errors.",
			Example:    `const packed = await codec.compression.compress("gzip", "hello world");`,
		},
		"compression.decompress": {
			Summary: "Decompress data previously produced by compress (same algorithm name required). Returns Uint8Array. Async.",
			Params: []scriptengine.Param{
				{Name: "algo", Type: "string", Desc: "Algorithm name (case-insensitive), matching the one used to compress: gzip / deflate / zlib / bzip2 / zstd / brotli / lz4 / xz / snappy."},
				{Name: "data", Type: "string | Uint8Array | ArrayBuffer", Desc: "Compressed input bytes."},
			},
			ReturnType: "Promise<Uint8Array>",
			Returns:    "Promise<Uint8Array> — the original decompressed bytes.",
			Errors:     "Throws if data is undefined/null or an unsupported type, the algorithm name is unknown, or the input is not valid for that algorithm.",
			Example: `const raw = await codec.compression.decompress("gzip", packed);
const text = new TextDecoder().decode(raw);`,
		},
		"barcode.formats": {
			Summary:    "Available encode formats (qr / datamatrix / aztec / pdf417 / code128 / code39 / codabar / ean13 / ean8 / upca).",
			ReturnType: "string[]",
			Returns:    "string[] — the ten symbology names accepted by barcode.encode.",
			Errors:     "Never throws.",
			Example:    `const fmts = codec.barcode.formats(); // ["qr", "datamatrix", ...]`,
		},
		"barcode.decodableFormats": {
			Summary:    "Available decode formats (qr / datamatrix / aztec / code128 / code39 / code93 / codabar / ean13 / ean8 / upca / upce / itf). PDF417 is encode-only.",
			ReturnType: "string[]",
			Returns:    "string[] — the twelve symbology names barcode.decode can recognise. PDF417 is absent (gozxing has no PDF417 decoder).",
			Errors:     "Never throws.",
			Example:    `const fmts = codec.barcode.decodableFormats();`,
		},
		"barcode.encode": {
			Summary: "Render data into a PNG of the chosen format. opts.width / opts.height default to 256x256 (2D) or 400x120 (1D). opts.quietZone (true or px count) pads a white margin — required for EAN/UPC to decode. Async.",
			Params: []scriptengine.Param{
				{Name: "format", Type: "string", Desc: "Symbology (case-insensitive): qr / datamatrix / aztec / pdf417 / code128 / code39 / codabar / ean13 / ean8 / upca."},
				{Name: "data", Type: "string", Desc: "Payload to encode. EAN/UPC require the exact digit count for the variant; the encoder validates content per symbology."},
				{Name: "opts", Type: "{ width?: number, height?: number, quietZone?: boolean | number }", Optional: true, Desc: "width / height set the output pixel dimensions (default 256x256 for 2D qr/datamatrix/aztec, 400x120 otherwise). quietZone pads a white margin: true uses 10% of width (min 10px), a number uses that many pixels per side, false/0/absent adds none. EAN/UPC need a quiet zone to be decodable."},
			},
			ReturnType: "Promise<Uint8Array>",
			Returns:    "Promise<Uint8Array> — PNG image bytes.",
			Errors:     "Throws if the format is unknown, the data is invalid for that symbology, or scaling / PNG encoding fails.",
			Example:    `const png = await codec.barcode.encode("qr", "https://example.com", { width: 320, height: 320 });`,
		},
		"barcode.decode": {
			Summary: "Decode a PNG/JPEG/WebP image to { format, text } via gozxing. Optional format hint skips the auto-detect walk. EAN/UPC need a quiet zone in the input. Async.",
			Params: []scriptengine.Param{
				{Name: "data", Type: "string | Uint8Array | ArrayBuffer", Desc: "Image bytes (PNG / JPEG / WebP). A string is treated as its raw UTF-8 bytes."},
				{Name: "format", Type: "string", Optional: true, Desc: "Symbology hint (case-insensitive) from decodableFormats. When given, only that reader runs; otherwise every decoder is tried in priority order and the first hit wins."},
			},
			ReturnType: "Promise<{ format: string, text: string }>",
			Returns:    "Promise<{ format: string, text: string }> — the detected symbology name and the decoded payload.",
			Errors:     "Throws if data is missing/empty or not a string/ArrayBuffer/Uint8Array, the image cannot be decoded, the format hint is unsupported, or no barcode is recognised.",
			Example:    `const { format, text } = await codec.barcode.decode(png);`,
		},
		"checkdigit.algos": {
			Summary:    "Supported algorithms (luhn / isbn10 / isbn13 / ean13 / ean8 / upca).",
			ReturnType: "string[]",
			Returns:    "string[] — the six supported algorithm names. isbn13 is an alias for ean13 (same check-digit math).",
			Errors:     "Never throws.",
			Example:    `const algos = codec.checkdigit.algos();`,
		},
		"checkdigit.validate": {
			Summary: "Return whether the input passes the named algorithm's check digit.",
			Params: []scriptengine.Param{
				{Name: "algo", Type: "string", Desc: "Algorithm name (case-insensitive, trimmed): luhn / isbn10 / isbn13 / ean13 / ean8 / upca."},
				{Name: "input", Type: "string", Desc: "The full number including its trailing check digit (whitespace trimmed). ISBN-10 may end in 'X'."},
			},
			ReturnType: "boolean",
			Returns:    "boolean — true when the input's check digit is valid for the algorithm. Returns false (does not throw) for wrong length, non-digit characters, or an unknown algorithm.",
			Errors:     "Never throws; any failure (unknown algorithm included) returns false.",
			Example:    `const ok = codec.checkdigit.validate("luhn", "4539578763621486"); // true`,
		},
		"checkdigit.compute": {
			Summary: "Compute the missing trailing check digit for a partial input.",
			Params: []scriptengine.Param{
				{Name: "algo", Type: "string", Desc: "Algorithm name (case-insensitive, trimmed): luhn / isbn10 / isbn13 / ean13 / ean8 / upca."},
				{Name: "partial", Type: "string", Desc: "The number WITHOUT its check digit (whitespace trimmed). Fixed-length algorithms expect exactly length-1 digits (e.g. 12 for ean13, 9 for isbn10)."},
			},
			ReturnType: "string",
			Returns:    "string — the single check digit ('0'–'9', or 'X' for isbn10 when the value is 10).",
			Errors:     "Throws if the algorithm is unknown, the input is empty / the wrong length, or contains a non-digit.",
			Example:    `const cd = codec.checkdigit.compute("ean13", "123456789012"); // "8"`,
		},
		"checkdigit.inspect": {
			Summary: "Diagnostic combining validate + compute: { valid, given, computed, … }.",
			Params: []scriptengine.Param{
				{Name: "algo", Type: "string", Desc: "Algorithm name (case-insensitive, trimmed): luhn / isbn10 / isbn13 / ean13 / ean8 / upca."},
				{Name: "input", Type: "string", Desc: "The full number including its trailing check digit (whitespace trimmed)."},
			},
			ReturnType: "{ algo: string, input: string, valid: boolean, given: string, computed: string }",
			Returns:    "{ algo, input, valid, given, computed } — algo/input echo the normalised arguments; given is the input's last character; computed is the recalculated check digit (empty when the input is too short or malformed to split); valid is true when given equals computed (case-insensitive).",
			Errors:     "Never throws; malformed input yields valid:false with an empty computed.",
			Example: `const r = codec.checkdigit.inspect("ean13", "1234567890128");
// { algo: "ean13", input: "...", valid: true, given: "8", computed: "8" }`,
		},
		"php.serialize": {
			Summary: "PHP serialize(): encode a value to PHP's canonical serialization string. Objects use the __class sentinel; cycles throw.",
			Params: []scriptengine.Param{
				{Name: "value", Type: "unknown", Desc: "Any JSON-like value: null, boolean, number, string, array, or plain object. An object carrying the class-key sentinel (opts.classKey, default \"__class\") encodes as a PHP object (O:)."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "classKey overrides the class-name sentinel property (default \"__class\"). indent / perlBoolClass are unused by serialize."},
			},
			ReturnType: "string",
			Returns:    "string — the PHP serialize() string (e.g. a:1:{...}).",
			Errors:     "Throws on a circular reference or an unsupported value type (function, Symbol, BigInt, Date, Map, Set, RegExp).",
			Example:    `const s = codec.php.serialize({ a: 1, b: [2, 3] });`,
		},
		"php.unserialize": {
			Summary: "PHP unserialize(): decode a serialize() string back to a value. r:/R: references resolve to shared objects (DAGs); cycles throw.",
			Params: []scriptengine.Param{
				{Name: "input", Type: "string", Desc: "A PHP serialize() string."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "classKey sets the sentinel property used to tag decoded PHP objects (default \"__class\")."},
			},
			ReturnType: "unknown",
			Returns:    "unknown — the decoded value. PHP objects become plain objects carrying the classKey sentinel; r:/R: references rebuild as shared object identities (DAGs).",
			Errors:     "Throws on malformed input or a reference that would close a cycle.",
			Example:    `const v = codec.php.unserialize('a:2:{i:0;i:1;i:1;i:2;}'); // [1, 2]`,
		},
		"php.varExport": {
			Summary: "PHP var_export(): emit valid PHP code for a value. opts.indent overrides the 2-space step.",
			Params: []scriptengine.Param{
				{Name: "value", Type: "unknown", Desc: "Any JSON-like value (see php.serialize). Objects with the class-key sentinel emit as \\Cls::__set_state(...)."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "indent overrides the default 2-space indentation step. classKey overrides the class-name sentinel (default \"__class\")."},
			},
			ReturnType: "string",
			Returns:    "string — valid PHP source, the kind var_export() prints.",
			Errors:     "Throws on a circular reference or an unsupported value type.",
			Example:    `const code = codec.php.varExport({ x: 1 }, { indent: "    " });`,
		},
		"php.parseVarExport": {
			Summary: "Read a var_export() literal (arrays, scalars, NULL, \\Cls::__set_state) back to a value.",
			Params: []scriptengine.Param{
				{Name: "input", Type: "string", Desc: "A PHP var_export() literal: array(...), scalars, NULL, or \\Cls::__set_state(array(...))."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "classKey sets the sentinel property used to tag decoded __set_state objects (default \"__class\")."},
			},
			ReturnType: "unknown",
			Returns:    "unknown — the decoded value; \\Cls::__set_state objects become plain objects carrying the classKey sentinel.",
			Errors:     "Throws on input that is not a parseable var_export() literal.",
			Example:    `const v = codec.php.parseVarExport("array (\n  0 => 1,\n)");`,
		},
		"php.varDump": {
			Summary: "PHP var_dump(): human-readable debug output. String lengths are byte counts.",
			Params: []scriptengine.Param{
				{Name: "value", Type: "unknown", Desc: "Any JSON-like value (see php.serialize)."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "indent overrides the default indentation step. classKey overrides the class-name sentinel (default \"__class\")."},
			},
			ReturnType: "string",
			Returns:    "string — var_dump()-style output. String lengths in the output are byte counts, matching PHP.",
			Errors:     "Throws on a circular reference or an unsupported value type.",
			Example:    `const dump = codec.php.varDump({ name: "ok" });`,
		},
		"php.parseVarDump": {
			Summary: "Best-effort read of var_dump() output. Throws on lossy markers (*RECURSION*, truncation, visibility-annotated props).",
			Params: []scriptengine.Param{
				{Name: "input", Type: "string", Desc: "PHP var_dump() output to parse back."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "classKey sets the sentinel property used to tag decoded objects (default \"__class\")."},
			},
			ReturnType: "unknown",
			Returns:    "unknown — the reconstructed value (best-effort; var_dump is a lossy format).",
			Errors:     "Throws on lossy markers it cannot faithfully reverse: *RECURSION*, truncation, or visibility-annotated (private/protected) properties.",
			Example:    `const v = codec.php.parseVarDump('int(42)'); // 42`,
		},
		"perl.dumper": {
			Summary: "Perl Data::Dumper-style dump ($VAR1 = … ;), normalized indentation. JS booleans emit the JSON::XS::Boolean blessed-ref form (opts.perlBoolClass).",
			Params: []scriptengine.Param{
				{Name: "value", Type: "unknown", Desc: "Any JSON-like value (see php.serialize). Arrays/objects emit as Perl array/hash refs; class-key objects emit as blessed refs."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "perlBoolClass names the blessed class emitted for JS booleans (default \"JSON::XS::Boolean\"). indent overrides the indentation step. classKey overrides the class-name sentinel (default \"__class\")."},
			},
			ReturnType: "string",
			Returns:    "string — a Data::Dumper-style dump ($VAR1 = ... ;) with normalized indentation.",
			Errors:     "Throws on a circular reference or an unsupported value type.",
			Example:    `const d = codec.perl.dumper({ ok: true });`,
		},
		"perl.parseDumper": {
			Summary: "Read Data::Dumper output back. Blessed scalar refs in the JSON bool family decode to booleans; bare 1/0 stay numbers; cycles throw.",
			Params: []scriptengine.Param{
				{Name: "input", Type: "string", Desc: "Data::Dumper output to parse back (a $VARn = ... ; assignment or a bare value)."},
				{Name: "opts", Type: "{ classKey?: string, perlBoolClass?: string, indent?: string }", Optional: true, Desc: "classKey sets the sentinel property used to tag decoded blessed refs (default \"__class\"). The JSON bool family (JSON::XS::Boolean, JSON::PP::Boolean, Types::Serialiser::Boolean) decodes to JS booleans regardless."},
			},
			ReturnType: "unknown",
			Returns:    "unknown — the decoded value. Blessed scalar refs in the JSON-bool family become JS booleans; bare 1/0 stay numbers; other blessed refs carry the classKey sentinel.",
			Errors:     "Throws on malformed input or a reference that would close a cycle.",
			Example:    `const v = codec.perl.parseDumper("$VAR1 = [1, 2];"); // [1, 2]`,
		},
		"xml.encode": {
			Summary: "Serialize a value to an XML string. Convention: @-prefixed keys are attributes, #text is element text, other keys are child elements, and an array value becomes repeated sibling elements (a scalar key becomes a text-only element, null a self-closing tag). The value must be a single-key object naming the root element, or pass opts.rootName to wrap it. Scalars are stringified; object key order is preserved.",
			Params: []scriptengine.Param{
				{Name: "value", Type: "unknown", Desc: "A single-key object whose one key names the root element — e.g. { note: { \"@id\": \"5\", \"#text\": \"hi\", to: \"alice\" } } → <note id=\"5\">hi<to>alice</to></note>. Or any value plus opts.rootName to wrap it. Cycles throw."},
				{Name: "opts", Type: "{ rootName?: string, indent?: string, declaration?: boolean }", Optional: true, Desc: "rootName wraps the value under that root element. indent pretty-prints with the given unit per level (default compact). declaration prepends <?xml version=\"1.0\" encoding=\"UTF-8\"?> (default off)."},
			},
			ReturnType: "string",
			Returns:    "The XML string.",
			Errors:     "Throws if the value has no single root element and no opts.rootName, if the root content is an array, if a non-scalar is used as an attribute or #text value, or if the value contains a cycle.",
			Example: `const xml = codec.xml.encode({ note: { "@id": "5", "#text": "hi" } });
// <note id="5">hi</note>`,
		},
		"xml.decode": {
			Summary: "Parse an XML string to a value using the same @-prefix + #text convention as xml.encode. Attributes become @-keys, text becomes #text (or a bare string for a text-only element), child elements become keys, and repeated same-name siblings become an array. Empty/self-closing elements decode to null. Namespace prefixes are kept literally; all values are strings (no type coercion). Mismatched tags, multiple roots, and malformed XML throw.",
			Params: []scriptengine.Param{
				{Name: "xml", Type: "string", Desc: "The XML document to parse."},
			},
			ReturnType: "unknown",
			Returns:    "A single-key object whose key is the root element name and whose value is the parsed content (key order follows document order; all leaf values are strings).",
			Errors:     "Throws on malformed XML, mismatched/mis-nested end tags, multiple root elements, or no root element.",
			Example: `const v = codec.xml.decode("<note id=\"5\">hi</note>");
// { note: { "@id": "5", "#text": "hi" } }`,
		},
	}
}
