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
#   clean    Remove built artifacts
#
# release and manual are intentionally separate from `build` so an
# interactive dev cycle doesn't pay their costs.

GO                ?= go
RECON             ?= recon
GOLANGCI_VERSION  ?= v2.12.2
BIN                = sercon
RELEASE_FLAGS      = -trimpath -ldflags=-s\ -w

.PHONY: build release manual test vet lint demo clean

DEMO_SCRIPTS = \
	examples/scripts/smoke.ts \
	examples/scripts/async.ts \
	examples/scripts/hash.ts \
	examples/scripts/strings.ts \
	examples/scripts/path-and-time.ts \
	examples/scripts/default-export.ts \
	examples/scripts/tsx-demo.ts

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

clean:
	rm -f $(BIN) MANUAL.pdf
