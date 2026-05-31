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
			Returns: "Promise<{ path: string, format: string, entries: string[], bytes?: number }> — path is destPath, format is the inferred format (\"zip\" | \"tar\" | \"tar.gz\"), entries lists the file paths written (directories excluded), and bytes is the final archive size when stat succeeds.",
			Errors:  "Rejects if destPath is empty, sources is not an array / is empty / contains a bad entry (object missing 'path', unsupported element type), the format cannot be inferred from the extension, or any disk read / write fails (e.g. a source path does not exist).",
			Example: `const r = await fs.archive.create("out.tar.gz", ["dist", { path: "README.md", name: "docs/README.md" }]);
runtime.log(r.format, r.entries.length);`,
		},
		"archive.extract": {
			Summary: "Extract a zip / tar / tar.gz to destDir. opts.overwrite controls O_EXCL behaviour.",
			Params: []scriptengine.Param{
				{Name: "archivePath", Type: "string", Desc: "Path to the archive. Format is inferred from its extension (.zip, .tar, .tar.gz, .tgz)."},
				{Name: "destDir", Type: "string", Desc: "Destination directory; created (recursively) if absent. All entries are confined to this directory via zip-slip / tar-slip protection."},
				{Name: "opts", Type: "{ overwrite?: boolean }", Optional: true, Desc: "overwrite (default false) clobbers existing files; when false, an entry colliding with an existing file fails the call (O_EXCL)."},
			},
			Returns: "Promise<{ path: string, format: string, dest: string, entries: string[] }> — path is archivePath, format is the inferred format, dest is destDir, and entries lists the extracted entry names (regular files only).",
			Errors:  "Rejects if archivePath or destDir is empty, the format cannot be inferred, destDir cannot be created, the archive cannot be opened / decoded, an entry escapes destDir (absolute path or '..' component), or (with overwrite false) an entry collides with an existing file.",
			Example: `const r = await fs.archive.extract("out.tar.gz", "./unpacked", { overwrite: true });
runtime.log(r.entries.length, "files extracted");`,
		},
	}
}
