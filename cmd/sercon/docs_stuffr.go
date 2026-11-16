// cmd/sercon/docs_stuffr.go
package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// Shared option shapes, kept as constants so the d.ts signatures stay in step.
const (
	stuffrCommonOpts = "cwd?: string; env?: Record<string, string>; stdin?: string | Uint8Array | ArrayBuffer; stream?: boolean; memoryLimit?: string | number"
	stuffrWriteOpts  = "force?: boolean; level?: number; allowWeakEncoder?: boolean; strictFidelity?: boolean"
)

const stuffrErrors = "Rejects with a stuffr error carrying `code` (the exit code), `kind` (\"failed\" 1, \"usage\" 2, \"unsupported\" 3, \"degraded\" 4 — output WAS written but something was lost or unverified, \"corrupt\" 5, \"limit\" 6, \"unsafe\" 7), `stderr` and `argv` when stuffr exits non-zero. Rejects with `kind: \"trap\"`, `argv` and `detail` (wazero's error text) if the wasm module traps (corrupt or hostile input, memory exhaustion, or a stuffr bug). Rejects on an invalid option type (e.g. \"option \\\"force\\\" must be a boolean\")."

// stuffrDocs documents the `stuffr` global (embedded stuffr under wazero).
func stuffrDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"run": {
			Summary: "Run the embedded stuffr with a raw argv and get the exit code and output back — never throws for a non-zero exit. argv[0] is a verb (pack, unpack, list, ...) or a personality name (gzip, unzip, zstdcat, ...), which then returns its reference tool's exit codes.",
			Params: []scriptengine.Param{
				{Name: "argv", Type: "string[]", Desc: "The stuffr command line, without a program name. A path argument that is exactly `-` means stdin; use `./-` to name a file called `-`."},
				{Name: "opts", Type: "{ cwd?: string; env?: Record<string, string>; stdin?: string | Uint8Array | ArrayBuffer; stream?: boolean }", Optional: true, Desc: "cwd: working directory the guest sees (default: the process cwd). env: extra guest environment variables (the host environment is not inherited). stdin: bytes fed to the guest. stream: also tee the guest's stderr to sercon's stderr (the console)."},
			},
			ReturnType: "{ code: number; stdout: Uint8Array; stderr: string; text(): string }",
			Returns:    "The guest's exit code, raw stdout bytes, stderr text, and text() decoding stdout as UTF-8.",
			Errors:     "Rejects for a malformed argv/options value, or with `kind: \"trap\"` (see the other members) if the wasm module traps. A non-zero exit is NOT an error here — inspect `code`.",
			Example:    "const r = await stuffr.run([\"list\", \"--json\", \"a.tar.gz\"]);\nif (r.code === 0) runtime.log(JSON.parse(r.text()));",
		},
		"pack": {
			Summary: "Pack files or directories into an archive or compressed file. The format comes from opts.format or the output extension.",
			Params: []scriptengine.Param{
				{Name: "inputs", Type: "string[]", Desc: "Paths to pack, relative to opts.cwd. Must not be empty."},
				{Name: "opts", Type: "{ output?: string; format?: string; " + stuffrWriteOpts + "; " + stuffrCommonOpts + " }", Optional: true, Desc: "output: destination path. format: e.g. \"tar.gz\", \"zip\". level: compression level (brotli's default level is slow; pass a lower one). allowWeakEncoder: required for zstd because the embedded pure-Rust zstd encoder is weak. force: overwrite. strictFidelity: exit 4 (kind \"degraded\") if anything would be lost or unverified. memoryLimit: stuffr's --memory-limit (xz/zstd on big inputs may need more than the 256 MiB WASI default)."},
			},
			ReturnType: "{ messages: string[] }",
			Returns:    "Resolves with { messages } once the work succeeded: the non-empty lines of stuffr's stderr (its one-line summary plus any `fidelity warning(s)` lines). Human-readable output for logging, NOT a stable contract to parse; it is never empty in practice. To detect metadata loss programmatically use strictFidelity: true and catch kind \"degraded\".",
			Errors:     stuffrErrors,
			Example:    "await stuffr.pack([\"src\"], { cwd: \"/work\", output: \"src.tar.gz\" });",
		},
		"unpack": {
			Summary: "Extract an archive or decompress a file. Extraction safety equals native stuffr: `../` and absolute entry paths, symlinked ancestors and unsafe symlink targets are refused with exit 7 (kind \"unsafe\"). Interim: modes, symlinks and owners are not applied (see the manual).",
			Params: []scriptengine.Param{
				{Name: "archive", Type: "string", Desc: "Archive path, relative to opts.cwd."},
				{Name: "opts", Type: "{ directory?: string; samePermissions?: boolean; force?: boolean; strictFidelity?: boolean; " + stuffrCommonOpts + " }", Optional: true, Desc: "directory: extraction destination (-C). samePermissions: restore recorded permissions (not effective in the embedded build yet). force: overwrite. strictFidelity: exit 4 (kind \"degraded\") if metadata would be lost, which makes the helper reject — the programmatic way to detect loss (the output is still written). memoryLimit: stuffr's --memory-limit."},
			},
			ReturnType: "{ messages: string[] }",
			Returns:    "Resolves with { messages } once the work succeeded: the non-empty lines of stuffr's stderr (its one-line summary plus any `fidelity warning(s)` lines). Human-readable output for logging, NOT a stable contract to parse; it is never empty in practice. To detect metadata loss programmatically use strictFidelity: true and catch kind \"degraded\".",
			Errors:     stuffrErrors + " A tar holding a symlink fails with exit 3 (\"unsupported\"); a file whose mode cannot be applied extracts with exit 0 and a fidelity warning, which is listed in `messages`, or — with strictFidelity: true — rejects with exit 4 (kind \"degraded\"; the output WAS written).",
			Example:    "try {\n  await stuffr.unpack(\"src.tar.gz\", { directory: \"/tmp/out\", strictFidelity: true });\n} catch (e: any) {\n  if (e.kind === \"degraded\") runtime.log(\"extracted, but metadata was lost:\", e.stderr);\n  else throw e;\n}",
		},
		"convert": {
			Summary: "Re-encode an archive or compressed file into another format without a manual unpack.",
			Params: []scriptengine.Param{
				{Name: "input", Type: "string", Desc: "Source path, relative to opts.cwd."},
				{Name: "output", Type: "string", Desc: "Destination path."},
				{Name: "opts", Type: "{ format?: string; " + stuffrWriteOpts + "; " + stuffrCommonOpts + " }", Optional: true, Desc: "format: target format (default: from the output extension). level, allowWeakEncoder, force, strictFidelity and memoryLimit as for pack."},
			},
			ReturnType: "{ messages: string[] }",
			Returns:    "Resolves with { messages } once the work succeeded: the non-empty lines of stuffr's stderr (its one-line summary plus any `fidelity warning(s)` lines). Human-readable output for logging, NOT a stable contract to parse; it is never empty in practice. To detect metadata loss programmatically use strictFidelity: true and catch kind \"degraded\".",
			Errors:     stuffrErrors,
			Example:    "await stuffr.convert(\"a.tar.gz\", \"a.tar.xz\");",
		},
		"test": {
			Summary: "Verify an archive's integrity (checksums and structure) without extracting it.",
			Params: []scriptengine.Param{
				{Name: "archive", Type: "string", Desc: "Archive path, relative to opts.cwd."},
				{Name: "opts", Type: "{ strictFidelity?: boolean; " + stuffrCommonOpts + " }", Optional: true, Desc: "strictFidelity: exit 4 (kind \"degraded\") if anything would be lost or unverified. cwd, env, stdin, stream and memoryLimit as for run."},
			},
			ReturnType: "{ messages: string[] }",
			Returns:    "Resolves with { messages } once the work succeeded: the non-empty lines of stuffr's stderr (its one-line summary plus any `fidelity warning(s)` lines). Human-readable output for logging, NOT a stable contract to parse; it is never empty in practice. To detect metadata loss programmatically use strictFidelity: true and catch kind \"degraded\".",
			Errors:     stuffrErrors + " A damaged archive rejects with kind \"corrupt\".",
			Example:    "const { messages } = await stuffr.test(\"backup.tar.zst\");\nruntime.log(messages.join(\"\\n\"));",
		},
		"cat": {
			Summary: "Decompress a single-stream compressed file (gzip, xz, zstd, ...) and return the bytes. Plain, uncompressed input is rejected by stuffr.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Compressed file path, relative to opts.cwd. A lone `-` means stdin (pass stdin in opts)."},
				{Name: "opts", Type: "{ " + stuffrCommonOpts + " }", Optional: true, Desc: "cwd, env, stdin, stream and memoryLimit as for run."},
			},
			ReturnType: "Uint8Array",
			Returns:    "The decompressed bytes.",
			Errors:     stuffrErrors,
			Example:    "const bytes = await stuffr.cat(\"notes.txt.gz\");",
		},
		"list": {
			Summary: "List an archive's entries as parsed JSON (runs `stuffr list --json`).",
			Params: []scriptengine.Param{
				{Name: "archive", Type: "string", Desc: "Archive path, relative to opts.cwd."},
				{Name: "opts", Type: "{ " + stuffrCommonOpts + " }", Optional: true, Desc: "cwd, env, stdin, stream and memoryLimit as for run."},
			},
			ReturnType: "unknown",
			Returns:    "The parsed JSON array, one object per entry — e.g. { index, kind: \"file\" | ..., name, size } — in stuffr's own `list --json` schema and key order.",
			Errors:     stuffrErrors + " Rejects with \"invalid JSON output\" if stuffr printed something unparseable.",
			Example:    "const entries: any[] = await stuffr.list(\"a.tar.gz\");\nruntime.log(entries.map((e) => e.name));",
		},
		"info": {
			Summary: "Describe an archive — format, sizes, compression — as parsed JSON (runs `stuffr info --json`).",
			Params: []scriptengine.Param{
				{Name: "archive", Type: "string", Desc: "Archive path, relative to opts.cwd."},
				{Name: "opts", Type: "{ " + stuffrCommonOpts + " }", Optional: true, Desc: "cwd, env, stdin, stream and memoryLimit as for run."},
			},
			ReturnType: "unknown",
			Returns:    "The parsed JSON object — e.g. { format, container, chain, bytes_in, rung, warnings, ... } — in stuffr's own `info --json` schema.",
			Errors:     stuffrErrors,
			Example:    "const i: any = await stuffr.info(\"a.tar.gz\");",
		},
		"formats": {
			Summary: "List the formats the embedded build supports, as parsed JSON (runs `stuffr formats --json`). The embedded build has the pure-Rust tier only.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ " + stuffrCommonOpts + " }", Optional: true, Desc: "Accepted for symmetry; none of the options changes the result."},
			},
			ReturnType: "unknown",
			Returns:    "The parsed JSON document; its `formats` array names each supported format.",
			Errors:     stuffrErrors,
			Example:    "const f: any = await stuffr.formats();\nruntime.log(f.formats.length);",
		},
		"version": {
			Summary:    "The version string of the embedded stuffr build (the first line of the committed VERSION file, e.g. a git describe plus build date). A plain string value, not a function.",
			ReturnType: "string",
			Returns:    "The embedded build's version identifier.",
			Errors:     "Never throws.",
			Example:    "runtime.log(stuffr.version);",
		},
	}
}
