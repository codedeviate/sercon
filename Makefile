# sercon Makefile
#
# Targets:
#   build    Debug build of ./cmd/sercon -> ./sercon
#   release  Stripped + trimpath build (~30% smaller); use this for shipping
#   manual   Render MANUAL.md -> MANUAL.pdf via the `recon` CLI (Chrome via
#            agent-browser is required at runtime)
#   test     go test ./...
#   vet      go vet ./...
#   lint     golangci-lint run against the whole repo (uses .golangci.yml).
#            If golangci-lint isn't on PATH, falls back to a one-shot
#            `go run` of the pinned version so contributors don't need to
#            install anything globally.
#   demo     Run every success-path example under examples/scripts/ so the
#            user-facing surface is exercised end-to-end. Excludes hang.ts
#            (intentional timeout that exits non-zero — verify separately).
#   types    Regenerate examples/scripts/api.d.ts from the current CLI
#            binding surface (the on-disk file is the source of truth for
#            editor autocomplete and the public api shape).
#   release-prep VERSION=x.y.z
#            Manual fallback: bump every version marker (Go const + MANUAL
#            cover + footer). Normally release-please does this in CI
#            (driven by Conventional Commits on master, see
#            release-please-config.json + .github/workflows/release-please.yml);
#            use this target only for ad-hoc local bumps when you're not
#            going through the release-please PR flow.
#   version-check
#            Verify the version markers in pkg/scriptengine/version.go and
#            MANUAL.md all agree. Run by release-prep; useful standalone
#            after editing one of the three by hand.
#   clean    Remove built artifacts
#
# release and manual are intentionally separate from `build` so an
# interactive dev cycle doesn't pay their costs.

GO                ?= go
RECON             ?= recon
GOLANGCI_VERSION  ?= v2.12.2
BIN                = sercon
RELEASE_FLAGS      = -trimpath -ldflags=-s\ -w

.PHONY: build release manual test vet lint demo types release-prep version-check clean

DEMO_SCRIPTS = \
	examples/scripts/smoke.ts \
	examples/scripts/async.ts \
	examples/scripts/hash.ts \
	examples/scripts/strings.ts \
	examples/scripts/path-and-time.ts \
	examples/scripts/default-export.ts \
	examples/scripts/tsx-demo.ts \
	examples/scripts/json-import.ts \
	examples/scripts/pkg-resolution.ts \
	examples/scripts/net-probe.ts \
	examples/scripts/email-auth.ts \
	examples/scripts/compression.ts \
	examples/scripts/barcode.ts \
	examples/scripts/charset.ts \
	examples/scripts/checkdigit.ts \
	examples/scripts/archive.ts \
	examples/scripts/diff.ts \
	examples/scripts/jq.ts \
	examples/scripts/exec-shell.ts \
	examples/scripts/exec-http.ts \
	examples/scripts/http-request.ts \
	examples/scripts/browser.ts \
	examples/scripts/git.ts \
	examples/scripts/gh.ts \
	examples/scripts/preg.ts \
	examples/scripts/preg2.ts \
	examples/scripts/jwt.ts \
	examples/scripts/encrypt.ts \
	examples/scripts/sqlite.ts \
	examples/scripts/redis.ts \
	examples/scripts/memcached.ts

build:
	CGO_ENABLED=0 $(GO) build -o $(BIN) ./cmd/sercon

release:
	CGO_ENABLED=0 $(GO) build $(RELEASE_FLAGS) -o $(BIN) ./cmd/sercon
	@ls -lh $(BIN) | awk '{print "  built:", $$NF, "(" $$5 ")"}'

manual:
	$(RECON) --md-to-pdf MANUAL.md -o MANUAL.pdf \
		--gfm --unsafe-html --page-break-on-h1 \
		--doc-title "sercon User Manual" \
		--doc-author "Thomas Bjork" \
		--doc-subject "Embeddable TypeScript script engine — reference and script-engine guide" \
		--doc-keywords "sercon, typescript, scripting, goja, esbuild, embedded"
# Note: --toc is intentionally omitted. MANUAL.md ships its own curated
# "## Table of contents" section (with the actual section numbers), and
# recon's auto-injected TOC lands above the cover-page <div>, pushing
# the cover to page 2. The curated TOC stays in flow and avoids that.

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./... ; \
	else \
		echo "golangci-lint not installed; falling back to one-shot go run @$(GOLANGCI_VERSION)"; \
		$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./... ; \
	fi

demo: build
	@./$(BIN) $(DEMO_SCRIPTS)
	@echo "All example scripts passed. (hang.ts is the timeout demo — run separately.)"

types: build
	./$(BIN) --emit-dts examples/scripts/api.d.ts

release-prep:
	@if [ -z "$(VERSION)" ]; then \
		echo "usage: make release-prep VERSION=x.y.z"; exit 2; \
	fi
	@echo "Bumping version markers to $(VERSION)..."
	@sed -i.bak -E 's/(^const Version = ")[^"]+(")/\1$(VERSION)\2/' pkg/scriptengine/version.go
	@sed -i.bak -E 's|(<div class="version">Version )[^<]+(</div>)|\1$(VERSION)\2|' MANUAL.md
	@sed -i.bak -E 's/(\*This manual covers sercon v)[0-9.]+(\.)/\1$(VERSION)\2/' MANUAL.md
	@sed -i.bak -E 's|(": ")[0-9.]+(")|\1$(VERSION)\2|' .release-please-manifest.json
	@rm -f pkg/scriptengine/version.go.bak MANUAL.md.bak .release-please-manifest.json.bak
	@$(MAKE) --no-print-directory version-check
	@echo ""
	@echo "Next steps:"
	@echo "  1) Edit CHANGELOG.md: move the [Unreleased] entries into [$(VERSION)] - $$(date +%Y-%m-%d)"
	@echo "  2) make manual && make types && make test && make vet && make lint && make demo"
	@echo "  3) git commit -am 'chore: cut v$(VERSION)'"
	@echo "  4) git tag -a v$(VERSION) -m 'release v$(VERSION)'"
	@echo "  5) git push origin master v$(VERSION)  # CI publishes binaries via goreleaser"

version-check:
	@const=$$(sed -nE 's|^const Version = "([^"]+)".*$$|\1|p' pkg/scriptengine/version.go); \
	cover=$$(sed -nE 's|.*<div class="version">Version ([^<]+)</div>.*|\1|p' MANUAL.md); \
	footer=$$(sed -nE 's|\*This manual covers sercon v([0-9.]+)\..*|\1|p' MANUAL.md); \
	manifest=$$(sed -nE 's|.*"\.": "([^"]+)".*|\1|p' .release-please-manifest.json); \
	if [ -z "$$const" ] || [ -z "$$cover" ] || [ -z "$$footer" ] || [ -z "$$manifest" ]; then \
		echo "version markers not found: code='$$const' cover='$$cover' footer='$$footer' manifest='$$manifest'"; \
		exit 1; \
	fi; \
	if [ "$$const" != "$$cover" ] || [ "$$const" != "$$footer" ] || [ "$$const" != "$$manifest" ]; then \
		echo "version mismatch: code=$$const cover=$$cover footer=$$footer manifest=$$manifest"; \
		exit 1; \
	fi; \
	echo "version markers in sync at $$const"

clean:
	rm -f $(BIN) MANUAL.pdf
