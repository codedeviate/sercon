# sercon Makefile
#
# Targets:
#   build    Debug build of ./cmd/sercon -> ./sercon
#   release  Stripped + trimpath build (~30% smaller); use this for shipping
#   manual   Render MANUAL.md -> MANUAL.pdf via the `recon` CLI (Chrome via
#            agent-browser is required at runtime)
#   test     go test ./...
#   vet      go vet ./...
#   clean    Remove built artifacts
#
# release and manual are intentionally separate from `build` so an
# interactive dev cycle doesn't pay their costs.

GO            ?= go
RECON         ?= recon
BIN            = sercon
RELEASE_FLAGS  = -trimpath -ldflags=-s\ -w

.PHONY: build release manual test vet clean

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

clean:
	rm -f $(BIN) MANUAL.pdf
