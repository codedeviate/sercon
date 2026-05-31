package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func codecDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"compression.algos":        {Summary: "Available compression algorithm names (gzip / deflate / zlib / bzip2 / zstd / brotli / lz4 / xz / snappy)."},
		"compression.compress":     {Summary: "Compress data with the named algorithm. Returns Uint8Array."},
		"compression.decompress":   {Summary: "Decompress data previously produced by compress (same algorithm name required)."},
		"barcode.formats":          {Summary: "Available encode formats (qr / datamatrix / aztec / pdf417 / code128 / code39 / codabar / ean13 / ean8 / upca)."},
		"barcode.decodableFormats": {Summary: "Available decode formats (qr / datamatrix / aztec / code128 / code39 / code93 / codabar / ean13 / ean8 / upca / upce / itf). PDF417 is encode-only."},
		"barcode.encode":           {Summary: "Render data into a PNG of the chosen format. opts.width / opts.height default to 256x256 (2D) or 400x120 (1D). opts.quietZone (true or px count) pads a white margin — required for EAN/UPC to decode."},
		"barcode.decode":           {Summary: "Decode a PNG/JPEG/WebP image to { format, text } via gozxing. Optional format hint skips the auto-detect walk. EAN/UPC need a quiet zone in the input."},
		"checkdigit.algos":         {Summary: "Supported algorithms (luhn / isbn10 / isbn13 / ean13 / ean8 / upca)."},
		"checkdigit.validate":      {Summary: "Return whether the input passes the named algorithm's check digit."},
		"checkdigit.compute":       {Summary: "Compute the missing trailing check digit for a partial input."},
		"checkdigit.inspect":       {Summary: "Diagnostic combining validate + compute: { valid, given, computed, … }."},
		"php.serialize":            {Summary: "PHP serialize(): encode a value to PHP's canonical serialization string. Objects use the __class sentinel; cycles throw."},
		"php.unserialize":          {Summary: "PHP unserialize(): decode a serialize() string back to a value. r:/R: references resolve to shared objects (DAGs); cycles throw."},
		"php.varExport":            {Summary: "PHP var_export(): emit valid PHP code for a value. opts.indent overrides the 2-space step."},
		"php.parseVarExport":       {Summary: "Read a var_export() literal (arrays, scalars, NULL, \\Cls::__set_state) back to a value."},
		"php.varDump":              {Summary: "PHP var_dump(): human-readable debug output. String lengths are byte counts."},
		"php.parseVarDump":         {Summary: "Best-effort read of var_dump() output. Throws on lossy markers (*RECURSION*, truncation, visibility-annotated props)."},
		"perl.dumper":              {Summary: "Perl Data::Dumper-style dump ($VAR1 = … ;), normalized indentation. JS booleans emit the JSON::XS::Boolean blessed-ref form (opts.perlBoolClass)."},
		"perl.parseDumper":         {Summary: "Read Data::Dumper output back. Blessed scalar refs in the JSON bool family decode to booleans; bare 1/0 stay numbers; cycles throw."},
	}
}
