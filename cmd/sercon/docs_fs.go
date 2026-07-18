package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func fsDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"path.dirname": {
			Summary:    "Directory portion of a path. POSIX-style; trailing slashes are stripped.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "A forward-slash path. On Windows, normalise separators yourself first."}},
			ReturnType: "string",
			Returns:    `string — everything up to (not including) the final slash; "." when the path has no directory component, "/" for a rooted single segment.`,
			Errors:     "Throws a TypeError if path is missing, null, or undefined.",
			Example:    `const d = fs.path.dirname("/var/log/app.log"); // "/var/log"`,
		},
		"path.basename": {
			Summary: "Final segment of a path; optional suffix is stripped if it matches.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "A forward-slash path. Trailing slashes are stripped before taking the last segment."},
				{Name: "suffix", Type: "string", Optional: true, Desc: "Trailing suffix to remove from the result (e.g. an extension). Only stripped when it matches and is not the entire segment; a non-matching or empty suffix is ignored."},
			},
			ReturnType: "string",
			Returns:    `string — the last path segment, with suffix removed when it applies.`,
			Errors:     "Throws a TypeError if path is missing, null, or undefined. suffix is coerced to a string and is optional.",
			Example:    `const b = fs.path.basename("/var/log/app.log", ".log"); // "app"`,
		},
		"archive.create": {
			Summary: "Create a zip / tar / tar.gz at destPath from a list of paths. Format inferred from extension.",
			Params: []scriptengine.Param{
				{Name: "destPath", Type: "string", Desc: "Output archive path. Format is inferred from the extension: .zip, .tar, .tar.gz, or .tgz."},
				{Name: "sources", Type: "(string | { path: string, name?: string })[]", Desc: "Non-empty array of inputs. A bare string uses the disk path as-is and its basename inside the archive; an object overrides the in-archive name via name. Directory sources are recursed (the directory's basename becomes the archive subdir). Archive paths always use forward slashes."},
			},
			ReturnType: "Promise<{ path: string; format: string; entries: string[]; bytes?: number }>",
			Returns:    "Promise<{ path: string, format: string, entries: string[], bytes?: number }> — path is destPath, format is the inferred format (\"zip\" | \"tar\" | \"tar.gz\"), entries lists the file paths written (directories excluded), and bytes is the final archive size when stat succeeds.",
			Errors:     "Rejects if destPath is empty, sources is not an array / is empty / contains a bad entry (object missing 'path', unsupported element type), the format cannot be inferred from the extension, or any disk read / write fails (e.g. a source path does not exist).",
			Example: `const r = await fs.archive.create("out.tar.gz", ["dist", { path: "README.md", name: "docs/README.md" }]);
runtime.log(r.format, r.entries.length);`,
		},
		"archive.extract": {
			Summary: "Extract a zip / tar / tar.gz to destDir. opts.overwrite controls O_EXCL behaviour; opts.maxTotalBytes/maxEntries cap decompression-bomb risk.",
			Params: []scriptengine.Param{
				{Name: "archivePath", Type: "string", Desc: "Path to the archive. Format is inferred from its extension (.zip, .tar, .tar.gz, .tgz)."},
				{Name: "destDir", Type: "string", Desc: "Destination directory; created (recursively) if absent. All entries are confined to this directory via zip-slip / tar-slip protection."},
				{Name: "opts", Type: "{ overwrite?: boolean, maxTotalBytes?: number, maxEntries?: number }", Optional: true, Desc: "overwrite (default false) clobbers existing files; when false, an entry colliding with an existing file fails the call (O_EXCL). maxTotalBytes caps the cumulative decompressed bytes written across all members (default 1 GB; a non-positive value falls back to the default). maxEntries caps the number of archive members processed (default 100000; a non-positive value falls back to the default). Both guard against a small crafted archive decompressing into an enormous amount of disk I/O (a decompression bomb); extraction stops as soon as either cap is exceeded."},
			},
			ReturnType: "Promise<{ path: string; format: string; dest: string; entries: string[] }>",
			Returns:    "Promise<{ path: string, format: string, dest: string, entries: string[] }> — path is archivePath, format is the inferred format, dest is destDir, and entries lists the extracted entry names (regular files only).",
			Errors:     "Rejects if archivePath or destDir is empty, the format cannot be inferred, destDir cannot be created, the archive cannot be opened / decoded, an entry escapes destDir (absolute path or '..' component), (with overwrite false) an entry collides with an existing file, the entry count exceeds maxEntries, or the cumulative decompressed size exceeds maxTotalBytes.",
			Example: `const r = await fs.archive.extract("out.tar.gz", "./unpacked", { overwrite: true, maxTotalBytes: 1 << 28, maxEntries: 5000 });
runtime.log(r.entries.length, "files extracted");`,
		},
		"writeText": {
			Summary: "Write a string to a file (UTF-8, truncating). Fails if the parent directory does not exist — call fs.mkdir first.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Output file path (CWD-relative or absolute). Used as given; no sandboxing."},
				{Name: "text", Type: "string", Desc: "Content to write as UTF-8. The file is created (mode 0644) or truncated."},
			},
			ReturnType: "Promise<{ path: string; bytes: number }>",
			Returns:    "Promise resolving to { path, bytes } where bytes is the number of bytes written.",
			Errors:     "Rejects if path is missing/empty, text is not a string, the parent directory does not exist, or the write fails (permissions, etc.).",
			Example: `await fs.mkdir("report");
await fs.writeText("report/index.html", "<h1>Hi</h1>");`,
		},
		"writeBytes": {
			Summary: "Write binary data (a Uint8Array) to a file, truncating. Fails if the parent directory does not exist.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Output file path (CWD-relative or absolute)."},
				{Name: "data", Type: "Uint8Array", Desc: "Bytes to write (mode 0644). Pass a Uint8Array (e.g. new Uint8Array(shot.bytes))."},
			},
			ReturnType: "Promise<{ path: string; bytes: number }>",
			Returns:    "Promise resolving to { path, bytes } where bytes is the number of bytes written.",
			Errors:     "Rejects if path is missing/empty, data is not a Uint8Array, the parent directory does not exist, or the write fails.",
			Example: `const shot = await d.screenshot();
await fs.writeBytes("shot.png", new Uint8Array(shot.bytes));`,
		},
		"readText": {
			Summary:    "Read an entire file as a UTF-8 string.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "File to read (CWD-relative or absolute)."}},
			ReturnType: "Promise<string>",
			Returns:    "Promise resolving to the file contents decoded as UTF-8.",
			Errors:     "Rejects if path is missing/empty or the file cannot be read (absent, permissions).",
			Example:    `const html = await fs.readText("report/index.html");`,
		},
		"readBytes": {
			Summary:    "Read an entire file as bytes.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "File to read (CWD-relative or absolute)."}},
			ReturnType: "Promise<Uint8Array>",
			Returns:    "Promise resolving to the file contents as a Uint8Array.",
			Errors:     "Rejects if path is missing/empty or the file cannot be read.",
			Example:    `const bytes = await fs.readBytes("shot.png");`,
		},
		"mkdir": {
			Summary:    "Create a directory, including any missing parents (mkdir -p). Idempotent.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "Directory path to create (mode 0755). Existing directories are fine."}},
			ReturnType: "Promise<{ path: string }>",
			Returns:    "Promise resolving to { path }.",
			Errors:     "Rejects if path is missing/empty or creation fails (e.g. a path component is an existing file).",
			Example:    `await fs.mkdir("report/assets");`,
		},
		"exists": {
			Summary:    "Report whether a path exists. Never throws for a missing path.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "Path to test."}},
			ReturnType: "Promise<boolean>",
			Returns:    "Promise resolving to true if the path exists, false if it does not.",
			Errors:     "Rejects only on an unexpected stat error (e.g. a permission error on a parent); a missing path resolves to false.",
			Example:    `if (!(await fs.exists("report"))) await fs.mkdir("report");`,
		},
		"remove": {
			Summary:    "Remove a file or a directory tree (recursive). No error if the path is already absent.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "File or directory to remove. Directories are removed recursively."}},
			ReturnType: "Promise<{ path: string }>",
			Returns:    "Promise resolving to { path }.",
			Errors:     "Rejects only if removal fails (e.g. permissions); removing an absent path is a no-op.",
			Example:    `await fs.remove("report"); // clean slate`,
		},
		"stat": {
			Summary:    "File metadata: size, whether it is a directory, and last-modified time.",
			Params:     []scriptengine.Param{{Name: "path", Type: "string", Desc: "Path to stat."}},
			ReturnType: "Promise<{ size: number; isDir: boolean; modifiedMs: number }>",
			Returns:    "Promise resolving to { size (bytes), isDir, modifiedMs (epoch milliseconds) }.",
			Errors:     "Rejects if path is missing/empty or the target does not exist.",
			Example: `const st = await fs.stat("report/index.html");
runtime.log(st.size, st.isDir, st.modifiedMs);`,
		},
		"find": {
			Summary: "Fast file/path search (fd-like): walk one or more roots and return matching paths. Respects .gitignore and skips hidden files by default (opt out with gitignore:false / hidden:true). Returns string[] of paths, or FindEntry[] with stat:true, or an async iterator with stream:true.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ root?: string | string[], glob?: string | string[], exclude?: string | string[], regex?: string, fullPath?: boolean, type?: \"file\"|\"dir\"|\"symlink\"|Array<\"file\"|\"dir\"|\"symlink\">, extension?: string | string[], case?: \"smart\"|\"sensitive\"|\"insensitive\", hidden?: boolean, gitignore?: boolean, followSymlinks?: boolean, maxDepth?: number, minDepth?: number, absolute?: boolean, limit?: number, sort?: boolean, strict?: boolean, stat?: boolean, stream?: boolean }", Optional: true, Desc: "Search options. root defaults to \".\". glob/exclude use ** globs; regex matches the basename (or, with fullPath, the path relative to root — which equals the displayed cwd-relative path only when root is \".\"). type/extension filter by kind. case defaults to smart (case-insensitive unless the regex has an uppercase char). hidden (default false) and gitignore (default true) control ignore-awareness. maxDepth/minDepth bound recursion. absolute returns absolute paths. limit caps results. sort yields deterministic path order. strict throws on the first traversal error (default: skip unreadable entries). stat returns { path, type, size, mtimeMs } objects. stream returns an async iterator."},
			},
			ReturnType: "Promise<string[]> | Promise<FindEntry[]> | AsyncIterable<string>",
			Returns:    "By default Promise<string[]> of matching paths (relative to cwd, or absolute with absolute:true). With stat:true, Promise<{ path: string, type: \"file\"|\"dir\"|\"symlink\", size: number, mtimeMs: number }[]>. With stream:true, an async iterator (for await ...) yielding the same items.",
			Errors:     "Throws (\"fs.find: …\") on an invalid regex, or on the first traversal error when strict:true. Unreadable files/dirs are skipped by default.",
			Example:    "const files = await fs.find({ glob: \"src/**/*.ts\", type: \"file\" });",
		},
		"grep": {
			Summary: "Fast content search (rg-like): walk roots (or explicit paths) and return per-line matches. RE2 regex by default (fixed:true for a literal). Respects .gitignore, skips hidden and binary files by default. Returns GrepMatch[], or an async iterator with stream:true.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ pattern: string, fixed?: boolean, root?: string | string[], paths?: string[], glob?: string | string[], exclude?: string | string[], type?: string | string[], extension?: string | string[], case?: \"smart\"|\"sensitive\"|\"insensitive\", word?: boolean, multiline?: boolean, context?: number, before?: number, after?: number, invert?: boolean, maxMatches?: number, maxResults?: number, includeBinary?: boolean, hidden?: boolean, gitignore?: boolean, followSymlinks?: boolean, maxDepth?: number, absolute?: boolean, sort?: boolean, strict?: boolean, stream?: boolean }", Desc: "pattern is a RE2 regex (or literal with fixed:true). root defaults to \".\"; paths searches an explicit list of files instead (directories in paths are not recursed). type maps rg-style names (ts/js/go/md/json/py/rs/c) to extensions. case defaults to smart. context (or before/after) adds context lines. invert emits non-matching lines. maxMatches caps per file; maxResults caps total. Binary files are skipped unless includeBinary. stream returns an async iterator."},
			},
			ReturnType: "Promise<GrepMatch[]> | AsyncIterable<GrepMatch>",
			Returns:    "Promise<{ path: string, line: number, column: number, match: string, text: string, before?: string[], after?: string[] }[]> (1-based line/column). With stream:true, an async iterator yielding the same objects.",
			Errors:     "Throws (\"fs.grep: …\") if pattern is missing/invalid, or on the first traversal/read error when strict:true. Unreadable and binary files are skipped by default.",
			Example:    "const hits = await fs.grep({ pattern: \"TODO\\\\(.*\\\\)\", type: \"ts\", context: 1 });",
		},
		"grepFiles": {
			Summary: "Like fs.grep but returns just the paths of files with at least one match (rg -l); one entry per file (deduplicated), no per-line detail.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "GrepOptions", Desc: "Same options as fs.grep (streaming/context options are ignored)."},
			},
			ReturnType: "Promise<string[]>",
			Returns:    "Promise<string[]> — paths (relative to cwd, or absolute with absolute:true) of files containing at least one match.",
			Errors:     "Throws (\"fs.grepFiles: …\") if pattern is missing/invalid, or on the first error when strict:true.",
			Example:    "const files = await fs.grepFiles({ pattern: \"deprecated\", type: \"go\" });",
		},
		"grepCount": {
			Summary: "Like fs.grep but returns per-file match counts (rg -c). Files with zero matches are omitted.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "GrepOptions", Desc: "Same options as fs.grep."},
			},
			ReturnType: "Promise<{ path: string; count: number }[]>",
			Returns:    "Promise<{ path: string, count: number }[]> — one entry per file with >= 1 match.",
			Errors:     "Throws (\"fs.grepCount: …\") if pattern is missing/invalid, or on the first error when strict:true.",
			Example:    "const counts = await fs.grepCount({ pattern: \"import\", type: \"ts\" });",
		},
	}
}
