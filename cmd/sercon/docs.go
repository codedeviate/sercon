package main

// Doc maps consumed by registerSurface via Engine.SetMemberDocsStructured to
// decorate the emitted d.ts with JSDoc blocks and drive the generated
// markdown reference. Each function returns docs for one top-level global;
// keys are member paths relative to that global (e.g. "log", "assert.equal",
// "hash.sha256"). A member documented with only a Summary behaves exactly
// like the previous flat doc string.
//
// When adding or changing a binding, update the matching entry here
// as part of the same change. Missing entries are silently tolerated
// by the emitter (no JSDoc block is rendered).
//
// The per-namespace doc functions now live in docs_<ns>.go files
// (docs_runtime.go, docs_crypto.go, docs_text.go, docs_codec.go,
// docs_fs.go, docs_net.go, docs_db.go, docs_services.go, docs_tui.go,
// docs_server.go, docs_console.go, docs_cloud.go) — one func <ns>Docs()
// per file.
