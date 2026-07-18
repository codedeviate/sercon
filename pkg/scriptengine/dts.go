package scriptengine

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// writeDTS emits a TypeScript declaration file for the host registrations.
// The mapping is intentionally small but covers the common host-binding
// shapes: functions, namespace-style maps, structs, and constructor funcs
// returning struct pointers. Unknown or self-referential types fall back to
// `unknown` rather than panicking or recursing forever.
//
// The output starts with a `// Reserved globals: …` line listing the
// registered top-level names so a reader can see the script-facing
// surface at a glance.
//
// `docs` is the engine's doc map (path → MemberDoc); paths use the
// dotted form ("log", "http", "http.get"). Bindings without a doc entry
// get no JSDoc block. Only the Summary is consumed here; the structured
// Params/Returns fields are used by other generators.
// dtsPrelude declares the ambient types that hand-written binding doc
// signatures reference by name (Request/Response for HTTP routes, Pane for
// exec.shell, Envelope/Message for SMTP, Image for image.encodeFrames, plus
// their dependencies). Without these the emitted .d.ts is not valid
// TypeScript (tsc: "Cannot find name 'Request'", …). Shapes are lifted from
// MANUAL.md (§5.6 server.http / server.smtp, §5.10 tui, §5.11 image); keep
// them in sync with the documented surface. The `--emit-dts`-then-`tsc`
// CI check guards against drift/omission.
const dtsPrelude = `// --- ambient types referenced by the binding signatures below ---
type CookieOpts = {
  domain?: string;
  path?: string;
  maxAge?: number;
  expires?: number;
  secure?: boolean;
  httpOnly?: boolean;
  sameSite?: "strict" | "lax" | "none";
};

type WSMessage =
  | { type: "text"; text: string }
  | { type: "binary"; bytes: Uint8Array };

type WebSocket = AsyncIterable<WSMessage> & {
  send(data: string | Uint8Array): Promise<void>;
  close(code?: number, reason?: string): Promise<void>;
  remote: string;
  closeCode?: number;
  closeReason?: string;
};

type SSEStream = {
  send(data: string | { data: unknown; event?: string; id?: string; retry?: number }): Promise<void>;
  close(): Promise<void>;
  readonly closed: Promise<void>;
  readonly remote: string;
};

type Request = {
  method: string;
  url: string;
  path: string;
  query: Record<string, string[]>;
  headers: Record<string, string[]>;
  params: Record<string, string>;
  body: string;
  bodyBytes: Uint8Array;
  remote: string;
  cookies: Record<string, string>;
};

type Response = {
  status(code: number): Response;
  header(name: string, value: string): Response;
  cookie(name: string, value: string, opts?: CookieOpts): Response;
  json(value: unknown): Response;
  text(s: string): Response;
  html(s: string): Response;
  bytes(b: Uint8Array, ct?: string): Response;
  empty(): Response;
  redirect(loc: string, code?: number): Response;
  upgradeWebSocket(opts?: { readBuffer?: number }): Promise<WebSocket>;
  sse(opts?: { keepAlive?: number; retry?: number }): SSEStream;
};

type Envelope = {
  from: string;
  recipients: string[];
  remote: string;
  helo: string;
  authenticatedUser?: string;
  tls?: { version: string; cipher: string };
};

type Message = {
  from: string;
  to: string[];
  cc: string[];
  subject: string;
  headers: Record<string, string[]>;
  body: { text: string; html: string };
  attachments: Array<{ filename: string; contentType: string; bytes: Uint8Array }>;
  raw: Uint8Array;
};

type Pane = {
  write(s: string): void;
  writeln(s: string): void;
  clear(): void;
  title(s: string): void;
};

type Image = {
  readonly width: number;
  readonly height: number;
  readonly format: string;
  resize(w: number, h: number, opts?: { filter?: "lanczos" | "nearest" | "linear" | "box" | "catmullrom" }): Image;
  fit(w: number, h: number): Image;
  thumbnail(w: number, h: number): Image;
  crop(x: number, y: number, w: number, h: number): Image;
  rotate(deg: number): Image;
  rotate90(): Image;
  rotate180(): Image;
  rotate270(): Image;
  flipH(): Image;
  flipV(): Image;
  orient(n: number): Image;
  brightness(pct: number): Image;
  contrast(pct: number): Image;
  saturation(pct: number): Image;
  gamma(g: number): Image;
  sharpen(sigma: number): Image;
  blur(sigma: number): Image;
  grayscale(): Image;
  invert(): Image;
  overlay(other: Image, x: number, y: number, opacity?: number): Image;
  paste(other: Image, x: number, y: number): Image;
  bytes(format: string, opts?: { quality?: number }): Uint8Array;
  save(path: string, opts?: { format?: string; quality?: number }): void;
};

type FindEntry = { path: string; type: "file" | "dir" | "symlink"; size: number; mtimeMs: number };

type GrepMatch = { path: string; line: number; column: number; match: string; text: string; before?: string[]; after?: string[] };

type GrepOptions = {
  pattern: string; fixed?: boolean; root?: string | string[]; paths?: string[];
  glob?: string | string[]; exclude?: string | string[]; type?: string | string[]; extension?: string | string[];
  case?: "smart" | "sensitive" | "insensitive"; word?: boolean; multiline?: boolean;
  context?: number; before?: number; after?: number; invert?: boolean;
  maxMatches?: number; maxResults?: number; includeBinary?: boolean;
  hidden?: boolean; gitignore?: boolean; followSymlinks?: boolean; maxDepth?: number;
  absolute?: boolean; sort?: boolean; strict?: boolean; stream?: boolean;
};

`

func writeDTS(w io.Writer, regs []registration, docs map[string]MemberDoc) error {
	bw := &errWriter{w: w}
	bw.WriteString("// AUTO-GENERATED by scriptengine.Engine.WriteTypes — do not edit by hand.\n")
	names := make([]string, 0, len(regs))
	for _, reg := range regs {
		names = append(names, reg.name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		bw.WriteString("// Reserved globals: (none).\n")
	} else {
		bw.WriteString("// Reserved globals: " + strings.Join(names, ", ") + ".\n")
	}
	bw.WriteString("\n")
	bw.WriteString(dtsPrelude)
	for _, reg := range regs {
		ctx := newTypeCtx()
		writeMemberJSDoc(bw, docs[reg.name], 0)
		switch reg.kind {
		case regValue:
			writeValueDecl(bw, ctx, reg.name, reg.value, docs[reg.name])
		case regNamespace:
			writeNamespaceDecl(bw, ctx, reg.name, reg.members, reg.value, docs)
		case regConstructor:
			writeConstructorDecl(bw, ctx, reg.name, reg.value)
		}
		bw.WriteString("\n")
	}
	return bw.err
}

// asyncReturnType picks the TS return type for a PromisifyAsync binding. A
// documented MemberDoc.ReturnType wins (it carries the rich resolved shape the
// AsyncBinding's marker type loses, which would otherwise render as
// `Promise<unknown>`); it is emitted verbatim when already `Promise<…>`-wrapped
// and wrapped otherwise. With no doc, it falls back to the marker's
// TSReturnType wrapped in Promise<…>, preserving the previous behaviour for
// undocumented async bindings.
func asyncReturnType(doc MemberDoc, a AsyncBinding) string {
	if doc.ReturnType != "" {
		if strings.HasPrefix(doc.ReturnType, "Promise<") {
			return doc.ReturnType
		}
		return "Promise<" + doc.ReturnType + ">"
	}
	return "Promise<" + a.TSReturnType + ">"
}

// sigFromParams renders a TS call signature from a MemberDoc's structured
// Params. Optional params get a trailing `?` on the name; param types are
// emitted verbatim from Param.Type (falling back to `unknown` when empty).
// `ret` is the return type to use — callers pass MemberDoc.Returns when set,
// otherwise the reflected return type the call site already computed.
func sigFromParams(params []Param, ret string) string {
	args := make([]string, 0, len(params))
	for _, p := range params {
		typ := p.Type
		if typ == "" {
			typ = "unknown"
		}
		// A rest parameter is encoded with a leading "..." on the type. TS
		// syntax places the "..." before the name (`...args: unknown[]`),
		// and a rest parameter is never optional.
		if rest, ok := strings.CutPrefix(typ, "..."); ok {
			args = append(args, "..."+p.Name+": "+rest)
			continue
		}
		name := p.Name
		if p.Optional {
			name += "?"
		}
		args = append(args, name+": "+typ)
	}
	return "(" + strings.Join(args, ", ") + "): " + ret
}

// escapeJSDoc neutralises embedded comment terminators so a doc string
// containing "*/" (e.g. "RS*/PS*/ES*", "media:*/dc:*") does not close the
// /** ... */ block early and turn the rest of the emitted .d.ts into
// syntactically invalid TypeScript.
func escapeJSDoc(s string) string {
	return strings.ReplaceAll(s, "*/", "*\\/")
}

// writeMemberJSDoc renders the JSDoc block for a documented member. When the
// MemberDoc carries Params/Returns it expands to a multi-line block with
// `@param` lines (one per param with a non-empty Desc) and an `@returns`
// line; otherwise it falls back to the plain Summary rendering.
func writeMemberJSDoc(w *errWriter, doc MemberDoc, indent int) {
	hasParams := len(doc.Params) > 0
	if !hasParams && doc.Returns == "" {
		writeJSDoc(w, doc.Summary, indent)
		return
	}
	pad := strings.Repeat("  ", indent)
	w.WriteString(pad + "/**\n")
	if s := strings.TrimSpace(doc.Summary); s != "" {
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimRight(line, " \t")
			if line == "" {
				w.WriteString(pad + " *\n")
				continue
			}
			w.WriteString(pad + " * " + escapeJSDoc(line) + "\n")
		}
	}
	for _, p := range doc.Params {
		if strings.TrimSpace(p.Desc) == "" {
			continue
		}
		w.WriteString(pad + " * @param " + p.Name + " " + escapeJSDoc(p.Desc) + "\n")
	}
	if doc.Returns != "" {
		w.WriteString(pad + " * @returns " + escapeJSDoc(doc.Returns) + "\n")
	}
	w.WriteString(pad + " */\n")
}

// writeJSDoc renders a `/** ... */` block at the given indent level.
// Single-line docs collapse to `/** doc */`; multi-line docs (split on
// `\n`) expand to a standard ` *`-prefixed block. No-op when doc is
// empty so the emitter can call it unconditionally.
func writeJSDoc(w *errWriter, doc string, indent int) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return
	}
	pad := strings.Repeat("  ", indent)
	if !strings.Contains(doc, "\n") {
		w.WriteString(pad + "/** " + escapeJSDoc(doc) + " */\n")
		return
	}
	w.WriteString(pad + "/**\n")
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			w.WriteString(pad + " *\n")
			continue
		}
		w.WriteString(pad + " * " + escapeJSDoc(line) + "\n")
	}
	w.WriteString(pad + " */\n")
}

type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) WriteString(s string) {
	if e.err != nil {
		return
	}
	_, e.err = io.WriteString(e.w, s)
}

// typeCtx tracks the set of types already being expanded so we can break
// cycles and limit recursion depth. The depth cap keeps generated output
// tractable for deeply nested types like goja's own internals.
type typeCtx struct {
	visiting map[reflect.Type]bool
	depth    int
}

const maxTypeDepth = 4

func newTypeCtx() *typeCtx { return &typeCtx{visiting: map[reflect.Type]bool{}} }

// enter marks t as in-flight and increments depth. Returns false if t is
// already being expanded or the depth cap has been hit; in that case nothing
// is recorded, so the caller must NOT pair the false result with a leave().
func (c *typeCtx) enter(t reflect.Type) bool {
	if c.depth >= maxTypeDepth {
		return false
	}
	if t != nil && c.visiting[t] {
		return false
	}
	if t != nil {
		c.visiting[t] = true
	}
	c.depth++
	return true
}

func (c *typeCtx) leave(t reflect.Type) {
	if t != nil {
		delete(c.visiting, t)
	}
	c.depth--
}

func writeValueDecl(w *errWriter, ctx *typeCtx, name string, value any, doc MemberDoc) {
	// Resolve RegisterFactory-style bindings by calling the factory with
	// nil vm/loop. The factory bodies build their value without
	// dereferencing those arguments (closures capture them for runtime),
	// so this is safe; a panic falls back to `unknown` with a TODO.
	if m, ok := value.(factoryMarker); ok {
		var recovered bool
		func() {
			defer func() {
				if r := recover(); r != nil {
					recovered = true
					w.WriteString(fmt.Sprintf("// TODO: factory %s panicked during introspection: %v\ndeclare const %s: unknown;\n", name, r, name))
				}
			}()
			value = m.fn(nil, nil)
		}()
		if recovered {
			return
		}
	}
	if a, ok := value.(AsyncBinding); ok {
		ret := asyncReturnType(doc, a)
		if len(doc.Params) > 0 {
			w.WriteString("declare function " + name + sigFromParams(doc.Params, ret) + ";\n")
			return
		}
		w.WriteString(fmt.Sprintf("declare function %s(...args: unknown[]): %s;\n", name, ret))
		return
	}
	t := reflect.TypeOf(value)
	if t == nil {
		w.WriteString(fmt.Sprintf("declare const %s: unknown;\n", name))
		return
	}
	switch t.Kind() {
	case reflect.Func:
		if len(doc.Params) > 0 || doc.ReturnType != "" {
			ret := doc.ReturnType
			if ret == "" {
				ret = returnType(ctx, t)
			}
			w.WriteString("declare function " + name + sigFromParams(doc.Params, ret) + ";\n")
			return
		}
		w.WriteString("declare function " + name + funcSig(ctx, t, false) + ";\n")
	case reflect.Struct, reflect.Pointer:
		w.WriteString(fmt.Sprintf("declare const %s: %s;\n", name, structShape(ctx, t)))
	default:
		w.WriteString(fmt.Sprintf("declare const %s: %s;\n", name, tsType(ctx, t)))
	}
}

func writeNamespaceDecl(w *errWriter, ctx *typeCtx, name string, members map[string]any, factory any, docs map[string]MemberDoc) {
	if m, ok := factory.(namespaceFactoryMarker); ok && members == nil {
		defer func() {
			if r := recover(); r != nil {
				w.WriteString(fmt.Sprintf("// TODO: factory %s panicked during introspection: %v\ndeclare const %s: { [key: string]: unknown };\n", name, r, name))
			}
		}()
		members = m.fn(nil, nil)
	}
	w.WriteString("declare const " + name + ": {\n")
	writeMemberObject(w, ctx, members, name, docs, 1)
	w.WriteString("};\n")
}

// writeMemberObject emits a namespace's member list. `path` is the
// namespace prefix used when looking up per-member docs; nested
// sub-namespaces extend it with their own key. Members are sorted
// alphabetically so the output is deterministic.
func writeMemberObject(w *errWriter, ctx *typeCtx, members map[string]any, path string, docs map[string]MemberDoc, indent int) {
	pad := strings.Repeat("  ", indent)
	keys := make([]string, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := members[k]
		memberPath := path + "." + k
		doc := docs[memberPath]
		writeMemberJSDoc(w, doc, indent)
		if a, ok := v.(AsyncBinding); ok {
			ret := asyncReturnType(doc, a)
			if len(doc.Params) > 0 {
				w.WriteString(pad + k + sigFromParams(doc.Params, ret) + ";\n")
				continue
			}
			w.WriteString(pad + k + "(...args: unknown[]): " + ret + ";\n")
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			w.WriteString(pad + k + ": {\n")
			writeMemberObject(w, ctx, nested, memberPath, docs, indent+1)
			w.WriteString(pad + "};\n")
			continue
		}
		t := reflect.TypeOf(v)
		switch {
		case t == nil:
			w.WriteString(pad + k + ": unknown;\n")
		case t.Kind() == reflect.Func:
			if len(doc.Params) > 0 || doc.ReturnType != "" {
				ret := doc.ReturnType
				if ret == "" {
					ret = returnType(ctx, t)
				}
				w.WriteString(pad + k + sigFromParams(doc.Params, ret) + ";\n")
				continue
			}
			w.WriteString(pad + k + funcSig(ctx, t, false) + ";\n")
		default:
			w.WriteString(pad + k + ": " + tsType(ctx, t) + ";\n")
		}
	}
}

func writeConstructorDecl(w *errWriter, ctx *typeCtx, name string, ctor any) {
	t := reflect.TypeOf(ctor)
	if t == nil || t.Kind() != reflect.Func {
		w.WriteString(fmt.Sprintf("// TODO: constructor %s has unexpected type\ndeclare class %s {}\n", name, name))
		return
	}
	w.WriteString("declare class " + name + " {\n")
	args := make([]string, 0, t.NumIn())
	for i := 0; i < t.NumIn(); i++ {
		args = append(args, fmt.Sprintf("arg%d: %s", i, tsType(ctx, t.In(i))))
	}
	w.WriteString("  constructor(" + strings.Join(args, ", ") + ");\n")
	if t.NumOut() >= 1 {
		ret := t.Out(0)
		methodsOf := ret
		if methodsOf.Kind() == reflect.Pointer {
			methodsOf = methodsOf.Elem()
		}
		if methodsOf.Kind() == reflect.Struct {
			for i := 0; i < ret.NumMethod(); i++ {
				m := ret.Method(i)
				w.WriteString("  " + lowerFirst(m.Name) + funcSig(ctx, m.Type, true) + ";\n")
			}
		}
	}
	w.WriteString("}\n")
}

func structShape(ctx *typeCtx, t reflect.Type) string {
	// Methods are reflected from the original type so pointer-receiver
	// methods on *T are picked up (Go exposes them on *T's method set, not
	// on T's). Fields, by contrast, live on the underlying struct.
	methodsT := t
	fieldsT := t
	if fieldsT.Kind() == reflect.Pointer {
		fieldsT = fieldsT.Elem()
	}
	if fieldsT.Kind() != reflect.Struct {
		return tsType(ctx, t)
	}
	if !ctx.enter(fieldsT) {
		return "unknown"
	}
	defer ctx.leave(fieldsT)

	var parts []string
	for i := 0; i < methodsT.NumMethod(); i++ {
		m := methodsT.Method(i)
		parts = append(parts, lowerFirst(m.Name)+funcSig(ctx, m.Type, true))
	}
	for i := 0; i < fieldsT.NumField(); i++ {
		f := fieldsT.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag := f.Tag.Get("json"); tag != "" && tag != "-" {
			if comma := strings.Index(tag, ","); comma >= 0 {
				tag = tag[:comma]
			}
			if tag != "" {
				name = tag
			}
		}
		parts = append(parts, name+": "+tsType(ctx, f.Type))
	}
	return "{ " + strings.Join(parts, "; ") + " }"
}

// funcSig formats a function signature. When isMethod is true, In(0) is the
// receiver (reflect.Method.Type includes it) and is skipped from the
// parameter list.
func funcSig(ctx *typeCtx, t reflect.Type, isMethod bool) string {
	start := 0
	if isMethod && t.NumIn() > 0 {
		start = 1
	}
	// goja host bindings often have the signature
	// `func(goja.FunctionCall) goja.Value`, which carries no useful type
	// information for callers in JS. Collapse those to `(...args: unknown[])`.
	if t.NumIn()-start == 1 && typeString(t.In(start)) == "goja.FunctionCall" {
		return "(...args: unknown[]): " + returnType(ctx, t)
	}
	args := make([]string, 0, t.NumIn()-start)
	for i := start; i < t.NumIn(); i++ {
		args = append(args, fmt.Sprintf("arg%d: %s", i-start, tsType(ctx, t.In(i))))
	}
	return "(" + strings.Join(args, ", ") + "): " + returnType(ctx, t)
}

func typeString(t reflect.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

func returnType(ctx *typeCtx, t reflect.Type) string {
	switch t.NumOut() {
	case 0:
		return "void"
	case 1:
		out := t.Out(0)
		if out == errorType {
			return "void"
		}
		return tsType(ctx, out)
	case 2:
		if t.Out(1) == errorType {
			return tsType(ctx, t.Out(0))
		}
		return "[" + tsType(ctx, t.Out(0)) + ", " + tsType(ctx, t.Out(1)) + "]"
	default:
		parts := make([]string, t.NumOut())
		for i := 0; i < t.NumOut(); i++ {
			parts[i] = tsType(ctx, t.Out(i))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// tsType maps a Go reflect.Type to a TypeScript type expression. It enforces
// the visited-set / depth-cap policy on types that can self-reference (funcs,
// structs, interfaces).
func tsType(ctx *typeCtx, t reflect.Type) string {
	if t == nil {
		return "unknown"
	}
	if t == errorType {
		return "Error"
	}
	// Treat goja.Value (an interface used everywhere in host bindings) as the
	// permissive TS `unknown`. Likewise FunctionCall when it shows up outside
	// a function parameter slot.
	switch typeString(t) {
	case "goja.Value":
		return "unknown"
	case "goja.FunctionCall":
		return "{ This: unknown; Arguments: unknown[] }"
	case "scriptengine.Ordered", "*scriptengine.Ordered":
		// Ordered is an insertion-ordered map used for results with
		// conditional/dynamic/decoded-JSON keys; its shape isn't known at
		// compile time, so the honest TS type is an object of unknowns.
		return "Record<string, unknown>"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return "Uint8Array"
		}
		return tsType(ctx, t.Elem()) + "[]"
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return "Record<string, " + tsType(ctx, t.Elem()) + ">"
		}
		return "Record<" + tsType(ctx, t.Key()) + ", " + tsType(ctx, t.Elem()) + ">"
	case reflect.Pointer:
		return tsType(ctx, t.Elem())
	case reflect.Interface:
		return "unknown"
	case reflect.Struct:
		// structShape handles its own enter/leave bookkeeping.
		return structShape(ctx, t)
	case reflect.Func:
		return funcSig(ctx, t, false)
	default:
		return "/* TODO: " + t.String() + " */ unknown"
	}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
