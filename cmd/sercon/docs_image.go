package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// imageHandleTS is the inline TypeScript type for the chainable Image handle
// returned by the module functions. The handle is a per-call goja object, so it
// is NOT introspectable by the d.ts generator — we spell its shape out here and
// use it as the verbatim ReturnType for the introspected module members
// (open/decode/rasterizeSVG). This keeps the emitted sercon.d.ts self-contained
// (no dangling named type), matching the inline-handle convention the other
// returned handles (e.g. net.raw.open) already use. Chainable transforms are
// typed as returning `unknown` because a self-referential inline object type
// cannot name itself; re-narrow with `as` when chaining, or consult the §image
// reference / MANUAL.md for the full method set.
const imageHandleTS = `{ ` +
	`readonly width: number; readonly height: number; readonly format: string; ` +
	`resize(width: number, height: number, opts?: { filter?: "lanczos" | "nearest" | "linear" | "box" | "catmullrom" }): unknown; ` +
	`fit(width: number, height: number): unknown; ` +
	`thumbnail(width: number, height: number): unknown; ` +
	`crop(x: number, y: number, w: number, h: number): unknown; ` +
	`rotate(degrees: number): unknown; ` +
	`rotate90(): unknown; rotate180(): unknown; rotate270(): unknown; ` +
	`flipH(): unknown; flipV(): unknown; ` +
	`brightness(percent: number): unknown; contrast(percent: number): unknown; ` +
	`gamma(gamma: number): unknown; saturation(percent: number): unknown; ` +
	`sharpen(sigma: number): unknown; blur(sigma: number): unknown; ` +
	`grayscale(): unknown; invert(): unknown; ` +
	`overlay(other: unknown, x: number, y: number, opacity?: number): unknown; ` +
	`paste(other: unknown, x: number, y: number): unknown; ` +
	`bytes(format: "png" | "jpeg" | "gif" | "tiff" | "bmp" | "webp", opts?: { quality?: number }): Uint8Array; ` +
	`save(path: string, opts?: { format?: string; quality?: number }): void` +
	` }`

// imageDocs documents the `image` global. Only the module-level members
// (open/decode/rasterizeSVG) are introspectable from the namespace factory, so
// they drive the emitted d.ts + §16 reference. The handle-method entries below
// describe the chainable Image returned by those functions; they are carried
// here for completeness and the long-form MANUAL.md prose (the runtime handle
// is built per-call and is not reflected into the generated surface).
func imageDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"open": {
			Summary: "Read an image file from disk and decode it into a chainable Image handle. The format is sniffed from the file's magic bytes (PNG/JPEG/GIF/TIFF/BMP/WebP), not the extension. GIF decodes the first frame only.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Filesystem path to the image file to read and decode."},
			},
			ReturnType: imageHandleTS,
			Returns:    "Image — a handle exposing read-only width/height/format and chainable transform methods.",
			Errors:     "Throws (\"image.open: …\") if the file cannot be read, or (\"image.decode: …\") if the bytes are not a recognised/decodable image.",
			Example:    `const im = image.open("avatar.jpg");`,
		},
		"decode": {
			Summary: "Decode in-memory image bytes into a chainable Image handle. The format is sniffed from the magic bytes (PNG/JPEG/GIF/TIFF/BMP/WebP); GIF decodes the first frame only.",
			Params: []scriptengine.Param{
				{Name: "data", Type: "Uint8Array", Desc: "The raw, encoded image bytes (e.g. from net.http, fs, or a clipboard read)."},
			},
			ReturnType: imageHandleTS,
			Returns:    "Image — a handle exposing read-only width/height/format and chainable transform methods.",
			Errors:     "Throws a TypeError if data is not a Uint8Array, or (\"image.decode: …\") if the bytes are not a recognised/decodable image.",
			Example:    `const im = image.decode(pngBytes);`,
		},
		"rasterizeSVG": {
			Summary: "Rasterize an SVG (a supported subset) to a raster Image at the requested pixel size. SVG is rasterize-IN only — there is no SVG output; the result is a raster image you then encode as PNG/JPEG/etc. The accepts a path string or in-memory SVG bytes.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "An SVG file path (string) or the raw SVG document bytes (Uint8Array)."},
				{Name: "opts", Type: "{ width: number; height: number }", Desc: "Target raster size in pixels; both width and height are required and must be > 0."},
			},
			ReturnType: imageHandleTS,
			Returns:    "Image — a raster handle (format reported as \"svg\") sized to opts.width × opts.height.",
			Errors:     "Throws a TypeError if src is neither a string nor Uint8Array, or if width/height are missing/non-positive; throws (\"image: svg parse: …\") on a malformed/unsupported SVG.",
			Example:    `const im = image.rasterizeSVG("logo.svg", { width: 256, height: 256 });`,
		},

		// --- Image handle methods (prose; not emitted into the generated surface) ---
		"resize": {
			Summary: "Resize to width × height with a resampling filter (default Lanczos). Passing 0 for one dimension preserves the aspect ratio (the other dimension is computed). Returns a fresh Image (immutable chaining).",
			Params: []scriptengine.Param{
				{Name: "width", Type: "number", Desc: "Target width in pixels. 0 means \"derive from height to preserve aspect\"."},
				{Name: "height", Type: "number", Desc: "Target height in pixels. 0 means \"derive from width to preserve aspect\"."},
				{Name: "opts", Type: "{ filter?: 'lanczos' | 'nearest' | 'linear' | 'box' | 'catmullrom' }", Optional: true, Desc: "Optional resampling filter; defaults to lanczos."},
			},
			ReturnType: "Image",
			Returns:    "Image — the resized image.",
			Errors:     "Does not throw on dimension; an unknown filter name falls back to lanczos.",
			Example:    `const thumb = im.resize(200, 0); // width 200, height auto`,
		},
		"fit": {
			Summary: "Scale the image down to fit entirely within a width × height box, preserving aspect ratio (no cropping; may letterbox conceptually). Returns a fresh Image.",
			Params: []scriptengine.Param{
				{Name: "width", Type: "number", Desc: "Maximum width of the bounding box, in pixels."},
				{Name: "height", Type: "number", Desc: "Maximum height of the bounding box, in pixels."},
			},
			ReturnType: "Image",
			Returns:    "Image — the largest image that fits within the box at the original aspect ratio.",
			Errors:     "Does not throw; non-positive or zero dimensions produce a degenerate (possibly empty) image rather than an error.",
			Example:    `const fitted = im.fit(800, 600);`,
		},
		"thumbnail": {
			Summary: "Produce an exactly width × height thumbnail by scaling and centre-cropping to fill the box (aspect ratio preserved, overflow cropped). Returns a fresh Image.",
			Params: []scriptengine.Param{
				{Name: "width", Type: "number", Desc: "Output width in pixels."},
				{Name: "height", Type: "number", Desc: "Output height in pixels."},
			},
			ReturnType: "Image",
			Returns:    "Image — a width × height thumbnail, centre-cropped to fill.",
			Errors:     "Does not throw; non-positive dimensions produce a degenerate image rather than an error.",
			Example:    `const sq = im.thumbnail(128, 128);`,
		},
		"crop": {
			Summary: "Crop a rectangular region at (x, y) of size w × h. Coordinates are validated against the image bounds. Returns a fresh Image.",
			Params: []scriptengine.Param{
				{Name: "x", Type: "number", Desc: "Left edge of the crop rectangle, in pixels."},
				{Name: "y", Type: "number", Desc: "Top edge of the crop rectangle, in pixels."},
				{Name: "w", Type: "number", Desc: "Crop width in pixels (> 0)."},
				{Name: "h", Type: "number", Desc: "Crop height in pixels (> 0)."},
			},
			ReturnType: "Image",
			Returns:    "Image — the cropped region.",
			Errors:     "Throws a TypeError when w/h are non-positive or the rectangle falls outside the image bounds.",
			Example:    `const region = im.crop(10, 10, 100, 100);`,
		},
		"rotate": {
			Summary: "Rotate counter-clockwise by an arbitrary angle (degrees) about the centre, filling exposed corners with transparency. Returns a fresh Image (its bounds grow to contain the rotation).",
			Params: []scriptengine.Param{
				{Name: "degrees", Type: "number", Desc: "Rotation angle in degrees, counter-clockwise."},
			},
			ReturnType: "Image",
			Returns:    "Image — the rotated image with transparent fill in the new corners.",
			Errors:     "Does not throw; any finite angle is accepted (multiples of 360 are a no-op).",
			Example:    `const tilted = im.rotate(15);`,
		},
		"rotate90":  {Summary: "Rotate 90° counter-clockwise (lossless, no resampling). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — rotated 90° CCW.", Errors: "Does not throw.", Example: `const r = im.rotate90();`},
		"rotate180": {Summary: "Rotate 180° (lossless). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — rotated 180°.", Errors: "Does not throw.", Example: `const r = im.rotate180();`},
		"rotate270": {Summary: "Rotate 270° counter-clockwise / 90° clockwise (lossless). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — rotated 270° CCW.", Errors: "Does not throw.", Example: `const r = im.rotate270();`},
		"flipH":     {Summary: "Flip horizontally (mirror left↔right). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — horizontally mirrored.", Errors: "Does not throw.", Example: `const m = im.flipH();`},
		"flipV":     {Summary: "Flip vertically (mirror top↔bottom). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — vertically mirrored.", Errors: "Does not throw.", Example: `const m = im.flipV();`},
		"brightness": {
			Summary:    "Adjust brightness by a percentage in [-100, 100]; positive brightens, negative darkens, 0 is a no-op. Returns a fresh Image.",
			Params:     []scriptengine.Param{{Name: "percent", Type: "number", Desc: "Brightness change in percent, -100 (black) .. 100 (white)."}},
			ReturnType: "Image",
			Returns:    "Image — brightness-adjusted.",
			Errors:     "Does not throw; values outside [-100, 100] are clamped to the boundary (e.g. 150 behaves identically to 100).",
			Example:    `const b = im.brightness(20);`,
		},
		"contrast": {
			Summary:    "Adjust contrast by a percentage in [-100, 100]; positive increases, negative flattens. Returns a fresh Image.",
			Params:     []scriptengine.Param{{Name: "percent", Type: "number", Desc: "Contrast change in percent, -100 .. 100."}},
			ReturnType: "Image",
			Returns:    "Image — contrast-adjusted.",
			Errors:     "Does not throw; values outside [-100, 100] are clamped to the boundary (e.g. 150 behaves identically to 100).",
			Example:    `const c = im.contrast(15);`,
		},
		"gamma": {
			Summary:    "Apply gamma correction. gamma < 1 darkens, gamma > 1 brightens, 1.0 is a no-op. Returns a fresh Image.",
			Params:     []scriptengine.Param{{Name: "gamma", Type: "number", Desc: "Gamma factor (> 0). 1.0 leaves the image unchanged."}},
			ReturnType: "Image",
			Returns:    "Image — gamma-corrected.",
			Errors:     "Does not throw; pass a positive gamma (a non-positive value yields an all-black or undefined result rather than an error).",
			Example:    `const g = im.gamma(1.2);`,
		},
		"saturation": {
			Summary:    "Adjust colour saturation by a percentage in [-100, 100]; -100 is fully desaturated (grayscale), positive boosts. Returns a fresh Image.",
			Params:     []scriptengine.Param{{Name: "percent", Type: "number", Desc: "Saturation change in percent, -100 (gray) .. 100."}},
			ReturnType: "Image",
			Returns:    "Image — saturation-adjusted.",
			Errors:     "Does not throw; values outside [-100, 100] are clamped to the boundary (e.g. 150 behaves identically to 100).",
			Example:    `const s = im.saturation(30);`,
		},
		"sharpen": {
			Summary:    "Sharpen via an unsharp-mask whose strength is the sigma of the underlying Gaussian. Returns a fresh Image.",
			Params:     []scriptengine.Param{{Name: "sigma", Type: "number", Desc: "Sharpening strength (Gaussian sigma); larger is stronger."}},
			ReturnType: "Image",
			Returns:    "Image — sharpened.",
			Errors:     "Does not throw; a sigma <= 0 leaves the image effectively unchanged.",
			Example:    `const s = im.sharpen(1.0);`,
		},
		"blur": {
			Summary:    "Gaussian blur with the given sigma (larger sigma → softer). Returns a fresh Image.",
			Params:     []scriptengine.Param{{Name: "sigma", Type: "number", Desc: "Gaussian blur sigma; larger blurs more."}},
			ReturnType: "Image",
			Returns:    "Image — blurred.",
			Errors:     "Does not throw; a sigma <= 0 leaves the image effectively unchanged.",
			Example:    `const soft = im.blur(2.0);`,
		},
		"grayscale": {Summary: "Convert to grayscale (luminance). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — grayscale.", Errors: "Does not throw.", Example: `const g = im.grayscale();`},
		"invert":    {Summary: "Invert colours (photographic negative). Returns a fresh Image.", ReturnType: "Image", Returns: "Image — colour-inverted.", Errors: "Does not throw.", Example: `const n = im.invert();`},
		"overlay": {
			Summary: "Composite another Image on top of this one at (x, y) with an optional opacity (alpha-blended). Returns a fresh Image the size of the base.",
			Params: []scriptengine.Param{
				{Name: "other", Type: "Image", Desc: "The overlay Image handle to draw on top."},
				{Name: "x", Type: "number", Desc: "Left offset of the overlay within the base, in pixels."},
				{Name: "y", Type: "number", Desc: "Top offset of the overlay within the base, in pixels."},
				{Name: "opacity", Type: "number", Optional: true, Desc: "Blend opacity 0..1 (default 1.0 = fully opaque)."},
			},
			ReturnType: "Image",
			Returns:    "Image — the base with the overlay alpha-blended on top.",
			Errors:     "Throws a TypeError if other is not an Image handle.",
			Example:    `const composed = base.overlay(watermark, 10, 10, 0.5);`,
		},
		"paste": {
			Summary: "Paste another Image at (x, y), replacing the base pixels (no blending). Returns a fresh Image the size of the base.",
			Params: []scriptengine.Param{
				{Name: "other", Type: "Image", Desc: "The Image handle to paste in."},
				{Name: "x", Type: "number", Desc: "Left offset of the paste, in pixels."},
				{Name: "y", Type: "number", Desc: "Top offset of the paste, in pixels."},
			},
			ReturnType: "Image",
			Returns:    "Image — the base with other pasted over it (opaque).",
			Errors:     "Throws a TypeError if other is not an Image handle.",
			Example:    `const stamped = base.paste(logo, 0, 0);`,
		},
		"bytes": {
			Summary: "Encode the image to bytes in the given format. quality (jpeg) is 1..100, default 90. WebP encode is lossless — the quality option is accepted but ignored.",
			Params: []scriptengine.Param{
				{Name: "format", Type: "'png' | 'jpeg' | 'gif' | 'tiff' | 'bmp' | 'webp'", Desc: "Target encode format."},
				{Name: "opts", Type: "{ quality?: number }", Optional: true, Desc: "Optional encode options. quality (1..100, default 90) applies to jpeg; ignored for png/gif/tiff/bmp and for the lossless webp encoder."},
			},
			ReturnType: "Uint8Array",
			Returns:    "Uint8Array — the encoded image bytes.",
			Errors:     "Throws (\"image: unsupported encode format …\") for an unknown format, or (\"image: webp encode: …\") if WebP encoding fails.",
			Example:    `const png = im.bytes("png");`,
		},
		"save": {
			Summary: "Encode and write the image to a file. The format is inferred from the path extension unless opts.format overrides it. quality (jpeg) is 1..100, default 90; WebP is lossless (quality ignored).",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Destination file path; its extension drives the format unless opts.format is set."},
				{Name: "opts", Type: "{ format?: string; quality?: number }", Optional: true, Desc: "Optional: override the encode format and/or set jpeg quality (1..100)."},
			},
			ReturnType: "void",
			Returns:    "void — writes the encoded image to disk.",
			Errors:     "Throws (\"image: cannot infer format from path …\") when the extension is unknown and no opts.format is given, or (\"image.save: …\") if the file cannot be written.",
			Example:    `im.resize(64, 64).save("out.png");`,
		},
	}
}
