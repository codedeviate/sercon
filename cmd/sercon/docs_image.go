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
	`orient(n: number): unknown; ` +
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
// they drive the emitted d.ts + §17 reference. The handle-method entries below
// describe the chainable Image returned by those functions; they are carried
// here for completeness and the long-form MANUAL.md prose (the runtime handle
// is built per-call and is not reflected into the generated surface).
func imageDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"open": {
			Summary: "Read an image file from disk and decode it into a chainable Image handle. The format is sniffed from the file's magic bytes (PNG/JPEG/GIF/TIFF/BMP/WebP), not the extension. GIF decodes the first frame only.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Filesystem path to the image file to read and decode."},
				{Name: "opts", Type: "{ autoOrient?: boolean }", Optional: true, Desc: "When autoOrient is true, the source's EXIF Orientation is read and the pixels are rotated upright; absent/unreadable Orientation is treated as a no-op (never throws)."},
			},
			ReturnType: imageHandleTS,
			Returns:    "Image — a handle exposing read-only width/height/format and chainable transform methods.",
			Errors:     "Throws (\"image.open: …\") if the file cannot be read, or (\"image.decode: …\") if the bytes are not a recognised/decodable image.",
			Example:    `const im = image.open("avatar.jpg", { autoOrient: true });`,
		},
		"decode": {
			Summary: "Decode in-memory image bytes into a chainable Image handle. The format is sniffed from the magic bytes (PNG/JPEG/GIF/TIFF/BMP/WebP); GIF decodes the first frame only.",
			Params: []scriptengine.Param{
				{Name: "data", Type: "Uint8Array", Desc: "The raw, encoded image bytes (e.g. from net.http, fs, or a clipboard read)."},
				{Name: "opts", Type: "{ autoOrient?: boolean }", Optional: true, Desc: "When autoOrient is true, the source's EXIF Orientation is read and the pixels are rotated upright; absent/unreadable Orientation is treated as a no-op (never throws)."},
			},
			ReturnType: imageHandleTS,
			Returns:    "Image — a handle exposing read-only width/height/format and chainable transform methods.",
			Errors:     "Throws a TypeError if data is not a Uint8Array, or (\"image.decode: …\") if the bytes are not a recognised/decodable image.",
			Example:    `const im = image.decode(pngBytes, { autoOrient: true });`,
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
		"orient": {
			Summary:    "Apply one of the 8 EXIF orientations to the raster pixels and return a fresh Image. n is the EXIF Orientation value (1=normal, 2=mirror-H, 3=180°, 4=mirror-V, 5/7=transpose/transverse, 6=90°CW, 8=90°CCW). 1 is a no-op copy. Pure raster — no EXIF is read (use open/decode { autoOrient: true } to drive this from a file's tag).",
			Params:     []scriptengine.Param{{Name: "n", Type: "number", Desc: "EXIF orientation value, an integer 1..8."}},
			ReturnType: "Image",
			Returns:    "Image — the reoriented raster (dimensions swap for n in 5..8).",
			Errors:     "Throws a TypeError if n is not an integer in 1..8.",
			Example:    "const up = image.decode(bytes).orient(6); // 90° clockwise",
		},
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

		"decodeFrames": {
			Summary: "Decode all frames of an animated image (GIF or APNG) into a raw, normalized frame model. Frames are returned as stored (sub-rectangles, not composited) with per-frame timing/placement metadata; the caller composites if needed. A non-animated image returns a single frame.",
			Params:  []scriptengine.Param{{Name: "src", Type: "string | Uint8Array", Desc: "Image path or encoded bytes (GIF/APNG/PNG/JPEG/…)."}},
			ReturnType: "{ format: string; width: number; height: number; loopCount: number; frames: { image: " + imageHandleTS + "; delayMs: number; xOffset: number; yOffset: number; disposal: \"none\" | \"background\" | \"previous\"; blend?: \"source\" | \"over\" }[] }",
			Returns: "An object with the container format/size/loopCount and a frames array; each frame's image is a chainable Image handle, delayMs the display time, xOffset/yOffset the placement, disposal the dispose method, and blend (APNG only) the blend op. loopCount 0 = loop forever.",
			Errors:  "Throws if the path can't be read or the bytes can't be decoded.",
			Example: "const a = image.decodeFrames(\"anim.gif\");\nruntime.log(a.format, a.frames.length, a.frames[0].delayMs);",
		},
		"encodeFrames": {
			Summary: "Encode a frame set into an animated GIF or APNG. Pass a spec shaped like decodeFrames' result (frames[], optional width/height/loopCount); choose the format via opts.format. GIF frames are palettized to 256 colors with Floyd–Steinberg dithering; APNG is full-color. Without opts.dest the encoded bytes are returned; with dest they're written to that path.",
			Params: []scriptengine.Param{
				{Name: "spec", Type: "{ width?: number; height?: number; loopCount?: number; frames: { image: Image; delayMs?: number; xOffset?: number; yOffset?: number; disposal?: \"none\" | \"background\" | \"previous\"; blend?: \"source\" | \"over\" }[] }", Desc: "The animation: a frames array (each with an Image handle + optional delayMs/offsets/disposal/blend) and optional canvas width/height (derived from frame extents when omitted) and loopCount (default 0 = forever)."},
				{Name: "opts", Type: "{ format?: \"gif\" | \"apng\"; dest?: string }", Optional: true, Desc: "format selects the encoder (default gif); dest, when set, writes the file and returns its path instead of bytes."},
			},
			ReturnType: "{ format: string; bytes?: Uint8Array; path?: string }",
			Returns:    "{ format, bytes } with the encoded animation, or { format, path } when opts.dest is set.",
			Errors:     "Throws if format is not gif/apng, if frames is empty, if a frame's image is not an Image handle, or on an encode/write failure.",
			Example:    "const a = image.decodeFrames(\"in.gif\");\nconst out = image.encodeFrames(a, { format: \"apng\" });\nruntime.log(out.format, out.bytes.length);",
		},

		// --- EXIF sub-namespace ---
		"exif.read": {
			Summary:    "Read an image's EXIF metadata into a grouped-by-IFD object (image/exif/gps/thumbnail). JPEG/PNG/TIFF return all tags; HEIC/AVIF/RAW return a curated subset. Returns {} when the image has no EXIF.",
			Params:     []scriptengine.Param{{Name: "src", Type: "string | Uint8Array", Desc: "Image path or raw bytes. String values are read from disk; Uint8Array values are parsed in-memory."}},
			ReturnType: "{ image?: Record<string, unknown>; exif?: Record<string, unknown>; gps?: Record<string, unknown>; thumbnail?: Record<string, unknown> }",
			Returns:    "An object grouped by IFD; rationals serialised as [num,den] arrays, GPS latitude/longitude as signed decimals, dates as EXIF strings, binary/undefined-type values as base64.",
			Errors:     "Throws if the path cannot be read or the image bytes cannot be parsed.",
			Example:    "const e = image.exif.read(\"photo.jpg\");\nruntime.log(e.image?.Make, e.gps?.GPSLatitude);",
		},
		"exif.write": {
			Summary: "Merge EXIF tags into an image (JPEG/PNG only). Existing tags not mentioned in data are preserved; pass null for a tag value to delete that tag. Returns {format, bytes} or writes to {format, path} when opts.dest is given.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "Image path or raw bytes. String values are read from disk; Uint8Array values are used directly."},
				{Name: "data", Type: "{ image?: Record<string, unknown>; exif?: Record<string, unknown>; gps?: Record<string, unknown>; thumbnail?: Record<string, unknown> }", Desc: "Tags to merge, grouped by IFD. A null value deletes that tag from the output."},
				{Name: "opts", Type: "{ dest?: string }", Optional: true, Desc: "Optional. When opts.dest is a path, the result is written there and {format, path} is returned; otherwise {format, bytes} is returned."},
			},
			ReturnType: "{ format: string; bytes: Uint8Array } | { format: string; path: string }",
			Returns:    "{format, bytes} with the updated image bytes when no dest, or {format, path} when opts.dest is given.",
			Errors:     "Throws if the format is not JPEG or PNG (unsupported write target), if src cannot be read, or if dest cannot be written.",
			Example:    "const out = image.exif.write(jpegBytes, { image: { Artist: \"Alice\" } });\n// out.bytes is the updated JPEG",
		},
		"exif.replace": {
			Summary: "Replace an image's entire EXIF block with the supplied tags (JPEG/PNG only). All pre-existing EXIF is discarded; only the tags in data are written. Returns {format, bytes} or writes to {format, path} when opts.dest is given.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "Image path or raw bytes. String values are read from disk; Uint8Array values are used directly."},
				{Name: "data", Type: "{ image?: Record<string, unknown>; exif?: Record<string, unknown>; gps?: Record<string, unknown>; thumbnail?: Record<string, unknown> }", Desc: "The complete new EXIF block grouped by IFD. Any tag not listed is absent from the output."},
				{Name: "opts", Type: "{ dest?: string }", Optional: true, Desc: "Optional. When opts.dest is a path, the result is written there and {format, path} is returned; otherwise {format, bytes} is returned."},
			},
			ReturnType: "{ format: string; bytes: Uint8Array } | { format: string; path: string }",
			Returns:    "{format, bytes} with the updated image bytes when no dest, or {format, path} when opts.dest is given.",
			Errors:     "Throws if the format is not JPEG or PNG (unsupported write target), if src cannot be read, or if dest cannot be written.",
			Example:    "const out = image.exif.replace(jpegBytes, { image: { Make: \"sercon\" } });\nconst e = image.exif.read(out.bytes); // e.image.Make === \"sercon\"",
		},
		"exif.clear": {
			Summary: "Remove all EXIF metadata from an image (JPEG/PNG only). Returns {format, bytes} with the stripped image bytes, or writes to {format, path} when opts.dest is given.",
			Params: []scriptengine.Param{
				{Name: "src", Type: "string | Uint8Array", Desc: "Image path or raw bytes. String values are read from disk; Uint8Array values are used directly."},
				{Name: "opts", Type: "{ dest?: string }", Optional: true, Desc: "Optional. When opts.dest is a path, the result is written there and {format, path} is returned; otherwise {format, bytes} is returned."},
			},
			ReturnType: "{ format: string; bytes: Uint8Array } | { format: string; path: string }",
			Returns:    "{format, bytes} with a stripped image (no EXIF) when no dest, or {format, path} when opts.dest is given.",
			Errors:     "Throws if the format is not JPEG or PNG (unsupported write target), if src cannot be read, or if dest cannot be written.",
			Example:    "const out = image.exif.clear(jpegBytes);\nconst e = image.exif.read(out.bytes); // e === {}",
		},

		// --- Steganography sub-namespace ---
		"stego.embed": {
			Summary: "Hide a payload inside a lossless image using least-significant-bit (LSB) steganography, returning PNG bytes. The carrier is decoded (PNG/JPEG/GIF/TIFF/BMP/WebP) and re-encoded as PNG (output is always PNG — re-encoding to a lossy format would destroy the hidden data). One bit is stored per R/G/B channel; the alpha channel is never modified (opaque carriers recommended). A non-empty password encrypts the payload with AES-256-GCM (PBKDF2-SHA256 key derivation). One to four bits are stored per R/G/B channel (the `bits` option, default 1); higher values raise capacity but make the change more visible and easier to detect.",
			Params: []scriptengine.Param{
				{Name: "carrier", Type: "string | Uint8Array", Desc: "The carrier image: a file path (string) or raw image bytes (Uint8Array)."},
				{Name: "payload", Type: "string | Uint8Array", Desc: "The data to hide. A string is stored as UTF-8 and marked as text (extract returns a string); a Uint8Array is stored as binary (extract returns a Uint8Array)."},
				{Name: "opts", Type: "{ password?: string; dest?: string; bits?: number }", Optional: true, Desc: "password: encrypt the payload with AES-256-GCM. dest: write the resulting PNG to this path instead of returning its bytes. bits: payload bits per channel, an integer 1..4 (default 1)."},
			},
			ReturnType: "{ bytes: Uint8Array } | { path: string }",
			Returns:    "An object with the PNG bytes ({ bytes }), or { path } when opts.dest was given.",
			Errors:     "Throws if the carrier cannot be decoded, if the payload is neither a string nor Uint8Array, if the payload exceeds the carrier capacity (\"payload too large (need N bytes, capacity M)\"), or if writing opts.dest fails.",
			Example:    `const out = image.stego.embed("cover.png", "meet at noon", { password: "s3cret" });`,
		},
		"stego.extract": {
			Summary:    "Recover a payload previously hidden by image.stego.embed. Reads the LSB stream, verifies the sercon stego header, and returns the payload as a string (if it was embedded as text) or a Uint8Array (if binary). If the payload was encrypted, the same password must be supplied; a wrong password fails the authentication check. The bit depth is read from the header, so no `bits` argument is needed.",
			Params: []scriptengine.Param{
				{Name: "carrier", Type: "string | Uint8Array", Desc: "The stego image (must be the lossless PNG produced by embed, or an identical copy): a file path or raw bytes."},
				{Name: "opts", Type: "{ password?: string }", Optional: true, Desc: "The password used at embed time, required when the payload was encrypted."},
			},
			ReturnType: "string | Uint8Array",
			Returns:    "The recovered payload — a string when embedded as text, otherwise a Uint8Array.",
			Errors:     "Throws if the carrier cannot be decoded, if no sercon stego payload is present (\"no sercon stego payload found\"), if the payload is truncated, if the payload is encrypted but no password is given, or if decryption fails (\"wrong password or corrupt data\").",
			Example:    `const msg = image.stego.extract("cover.png", { password: "s3cret" });`,
		},
		"stego.capacity": {
			Summary:    "Report the maximum payload size (in bytes) a carrier can hold, after the fixed 10-byte header — one bit per R/G/B channel. Encryption adds roughly 44 bytes of overhead (salt + nonce + auth tag), so the effective capacity for an encrypted payload is correspondingly lower.",
			Params: []scriptengine.Param{
				{Name: "carrier", Type: "string | Uint8Array", Desc: "The carrier image: a file path or raw image bytes."},
				{Name: "opts", Type: "{ bits?: number }", Optional: true, Desc: "bits: report capacity at this depth (integer 1..4, default 1)."},
			},
			ReturnType: "{ bytes: number; bits: number }",
			Returns:    "An object: bytes is the maximum plaintext payload size at the requested depth; bits echoes that depth.",
			Errors:     "Throws if the carrier cannot be decoded, or a TypeError if bits is not an integer 1..4.",
			Example:    `const room = image.stego.capacity("cover.png", { bits: 4 }).bytes;`,
		},
		"stego.detect": {
			Summary: "Quickly check whether an image carries hidden data. Returns a definitive flag for a sercon-format LSB payload (the \"ScSt\" header) plus a heuristic suspicion verdict from statistical analysis. Read-only; never modifies the image.",
			Params: []scriptengine.Param{
				{Name: "carrier", Type: "string | Uint8Array", Desc: "The image to inspect: a file path or raw image bytes."},
			},
			ReturnType: "{ sercon: boolean; encrypted?: boolean; text?: boolean; payloadBytes?: number; bits?: number; suspicious: boolean; confidence: number }",
			Returns:    "An object: sercon is true when a sercon stego header is present (with encrypted/text/payloadBytes); suspicious/confidence summarize the statistical analysis. bits is the declared payload depth when a sercon header is present.",
			Errors:     "Throws if the carrier cannot be decoded. Never throws for a clean image.",
			Example:    `if (image.stego.detect("photo.png").sercon) console.log("hidden payload!");`,
		},
		"stego.analyze": {
			Summary: "Run a full LSB-steganalysis report on an image: per-channel chi-square (pairs-of-values) probability, LSB-plane Shannon entropy, and an RS embedding-rate estimate, plus a combined verdict with reasons. Read-only. The report includes a generalized chi-square at depths 1..4 (chiSquareByBits), per-plane entropy (entropyByPlane, planes 0..3), and an estimatedBits figure for the likely embedding depth.",
			Params: []scriptengine.Param{
				{Name: "carrier", Type: "string | Uint8Array", Desc: "The image to analyze: a file path or raw image bytes."},
			},
			ReturnType: "{ width: number; height: number; capacity: number; estimatedBits: number; sercon: { present: boolean; encrypted?: boolean; text?: boolean; payloadBytes?: number; bits?: number }; channels: { channel: string; chiSquare: number; lsbEntropy: number; rsEstimate: number; chiSquareByBits: number[]; entropyByPlane: number[] }[]; verdict: { suspicious: boolean; confidence: number; reasons: string[] } }",
			Returns:    "A report object: capacity in bytes, the sercon-header check, per-channel signals (chiSquare/lsbEntropy/rsEstimate, each 0..1), and the verdict. estimatedBits is a coarse, best-effort hint at the embedding depth — it needs substantial coverage to register and returns 0 for small or partial payloads. For sercon-format payloads the authoritative depth is sercon.bits (set by the header); chiSquareByBits and entropyByPlane are the raw per-depth diagnostics.",
			Errors:     "Throws if the carrier cannot be decoded.",
			Example:    `const r = image.stego.analyze("suspect.png"); console.log(r.verdict.suspicious, r.verdict.reasons);`,
		},
		"stego.bitplane": {
			Summary: "Render one bit-plane of an image as a PNG for visual inspection. Hidden LSB data often shows as structured noise in the low planes. For a single channel a set bit is white and an unset bit black; for \"rgb\" each channel's bit maps to its colour component.",
			Params: []scriptengine.Param{
				{Name: "carrier", Type: "string | Uint8Array", Desc: "The image: a file path or raw image bytes."},
				{Name: "opts", Type: `{ channel?: "r" | "g" | "b" | "rgb"; plane?: number; dest?: string }`, Optional: true, Desc: "channel selects the colour channel (default \"rgb\" composite); plane is the bit index 0 (LSB) to 7 (MSB), default 0; dest writes the PNG to that path instead of returning bytes."},
			},
			ReturnType: "{ bytes: Uint8Array } | { path: string }",
			Returns:    "An object with the PNG bytes ({ bytes }), or { path } when opts.dest was given.",
			Errors:     "Throws a TypeError for an unknown channel or a plane outside 0..7, throws if the carrier cannot be decoded, or if writing opts.dest fails.",
			Example:    `const lsb = image.stego.bitplane("photo.png", { plane: 0 });`,
		},
	}
}
