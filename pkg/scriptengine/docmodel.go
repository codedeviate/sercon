package scriptengine

// Param describes a single parameter of a documented binding member. It is
// the structured counterpart to a hand-written `@param` line: the d.ts
// emitter and the markdown reference generator both consume it to render a
// real signature and a parameter description.
type Param struct {
	// Name is the parameter's identifier as it appears in the signature.
	Name string
	// Type is the TS-facing type string, e.g. "string", "number",
	// "{ algorithm?: string }". It is emitted verbatim into the d.ts
	// signature, so it must be valid TypeScript.
	Type string
	// Optional marks the parameter as optional (rendered with a trailing
	// `?` in the signature).
	Optional bool
	// Desc is a short human description of the parameter, used for the
	// `@param` line and the markdown reference's parameter list.
	Desc string
}

// MemberDoc is the structured documentation for a single registered binding
// member. It is the single source of truth that drives both the generated
// `.d.ts` signatures (`@param`/return shapes) and the generated markdown
// binding reference. A member documented with only a Summary behaves exactly
// like the previous flat doc string: the d.ts emitter falls back to the
// reflected `(...args)` signature.
type MemberDoc struct {
	// Summary is the one-line description (what the previous flat
	// map[string]string held). It renders as the JSDoc summary line.
	Summary string
	// Params describes each parameter. When empty, the d.ts emitter falls
	// back to the reflected `(...args: unknown[])` signature.
	Params []Param
	// ReturnType is the bare TS return type emitted into the d.ts signature
	// (e.g. "string", "{ valid: boolean }"). It must be valid TypeScript —
	// do NOT put prose here (that's Returns). Empty falls back to the
	// reflected Go return type. Ignored for async bindings (their Promise<T>
	// type comes from the binding's Promised[T] marker).
	ReturnType string
	// Returns is prose describing the return value, for the `@returns` JSDoc
	// line and the markdown reference's Returns section (NOT the signature).
	Returns string
	// Errors describes when and what the member throws.
	Errors string
	// Example is a short TS snippet (no fences; generators add them).
	Example string
}
