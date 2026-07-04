package scriptengine

// Version is the released version of the scriptengine library and the
// bundled sercon CLI. Bumped by hand via `make release-prep VERSION=x.y.z`
// (see CLAUDE.md § Versioning and commits); the `x-release-please-version`
// marker below is a leftover from the release-please era, kept only because
// `make release-prep` still locates this line by it. The git tag (with `v`
// prefix) is created and pushed manually by the maintainer after the cut.
const Version = "0.86.0" // x-release-please-version
