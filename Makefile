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
#   types    Regenerate examples/scripts/sercon.d.ts from the current CLI
#            binding surface (the on-disk file is the source of truth for
#            editor autocomplete and the public api shape).
#   release-prep VERSION=x.y.z
#            Bump every version marker (Go const + MANUAL cover + footer +
#            HISTORY.md span line)
#            in one shot, then print the next-step checklist. Releases are
#            cut manually: this + edit CHANGELOG + make manual + tag + push.
#            See CLAUDE.md > Versioning and commits.
#   version-check
#            Verify the version markers in pkg/scriptengine/version.go and
#            MANUAL.md all agree. Run by release-prep; useful standalone
#            after editing one of the two by hand.
#   homebrew-bump VERSION=x.y.z
#            Bump the Homebrew formula in the codedeviate/homebrew-cli tap
#            (HOMEBREW_TAP) by running that tap's scripts/bump.sh, then commit
#            + push the formula. Run AFTER `git push` of the vX.Y.Z tag — the
#            bump fetches the release tarball to recompute its sha256.
#   clean    Remove built artifacts
#
# release and manual are intentionally separate from `build` so an
# interactive dev cycle doesn't pay their costs.

GO                ?= go
RECON             ?= recon
HOMEBREW_TAP      ?= /Users/thomas/Development/Thomas/Rust/homebrew-cli
RELEASE_REPO      ?= codedeviate/sercon
GOLANGCI_VERSION  ?= v2.12.2
BIN                = sercon
RELEASE_FLAGS      = -trimpath -ldflags=-s\ -w
MANUAL_VERSION    := $(shell sed -nE 's|^const Version = "([^"]+)".*|\1|p' pkg/scriptengine/version.go)
MANUAL_DATE       := $(shell date +%F)

.PHONY: build release manual reference test test-integration vet lint fmt fmt-check demo types release-prep release-verify version-check homebrew-bump paymentproviders paymentproviders-check sample-data clean favro favro-check

DEMO_SCRIPTS = \
	examples/scripts/smoke.ts \
	examples/scripts/async.ts \
	examples/scripts/argv.ts \
	examples/scripts/console.ts \
	examples/scripts/deadline.ts \
	examples/scripts/secrets.ts \
	examples/scripts/clipboard.ts \
	examples/scripts/hash.ts \
	examples/scripts/strings.ts \
	examples/scripts/path-and-time.ts \
	examples/scripts/default-export.ts \
	examples/scripts/export-default.ts \
	examples/scripts/tsx-demo.ts \
	examples/scripts/json-import.ts \
	examples/scripts/pkg-resolution.ts \
	examples/scripts/net-probe.ts \
	examples/scripts/traceroute.ts \
	examples/scripts/raw.ts \
	examples/scripts/email-auth.ts \
	examples/scripts/compression.ts \
	examples/scripts/barcode.ts \
	examples/scripts/charset.ts \
	examples/scripts/checkdigit.ts \
	examples/scripts/env.ts \
	examples/scripts/dump-codec.ts \
	examples/scripts/codec-xml.ts \
	examples/scripts/codec-toml.ts \
	examples/scripts/codec-yaml.ts \
	examples/scripts/archive.ts \
	examples/scripts/diff.ts \
	examples/scripts/jq.ts \
	examples/scripts/exec-shell.ts \
	examples/scripts/exec-stream.ts \
	examples/scripts/async-timer-host.ts \
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
	examples/scripts/valkey.ts \
	examples/scripts/memcached.ts \
	examples/scripts/ldap.ts \
	examples/scripts/dict.ts \
	examples/scripts/ai.ts \
	examples/scripts/server-http.ts \
	examples/scripts/server-static.ts \
	examples/scripts/server-ws.ts \
examples/scripts/server-sse.ts \
	examples/scripts/server-smtp.ts \
	examples/scripts/server-tcp.ts \
	examples/scripts/server-icmp.ts \
	examples/scripts/mcp-server-http.ts \
	examples/scripts/mcp-toolbox.ts \
	examples/scripts/mcp-bridge.ts \
	examples/scripts/mcp-resources.ts \
	examples/scripts/mcp-progress.ts \
	examples/scripts/mcp-subscribe.ts \
	examples/scripts/mcp-client-http.ts \
	examples/scripts/net-sockets.ts \
	examples/scripts/load.ts \
	examples/scripts/capture-file.ts \
	examples/scripts/image.ts \
	examples/scripts/image-anim.ts \
	examples/scripts/exif.ts \
	examples/scripts/stego.ts \
	examples/scripts/stego-analyze.ts \
	examples/scripts/text-stego.ts \
	examples/scripts/text-markdown.ts \
	examples/scripts/audio-stego.ts \
	examples/scripts/audio-format.ts \
	examples/scripts/tui-keys.ts \
	examples/scripts/agent-browser-core.ts \
	examples/scripts/agent-browser-capture.ts \
	examples/scripts/agent-browser-state.ts \
	examples/scripts/agent-browser-advanced.ts \
	examples/scripts/agent-browser-frames.ts \
	examples/scripts/webdriver.ts \
	examples/scripts/webdriver-advanced.ts \
	examples/scripts/webdriver-frames.ts \
	examples/scripts/webdriver-wait-click.ts \
	examples/scripts/webdriver-cdp-click.ts \
	examples/scripts/webdriver-cdp-oopif.ts \
	examples/scripts/fs-report.ts \
	examples/scripts/fs-find.ts \
	examples/scripts/typst.ts \
	examples/scripts/pdf-extract.ts \
	examples/scripts/doctor.ts \
	examples/scripts/paymentproviders-kcov3.ts \
	examples/scripts/paymentproviders-nets.ts \
	examples/scripts/paymentproviders-svea.ts \
	examples/scripts/paymentproviders-qliro.ts \
	examples/scripts/paymentproviders-swedbankpay.ts \
	examples/scripts/favro.ts \
	examples/scripts/advanced/load-resilience.ts \
	examples/scripts/advanced/http-api.ts \
	examples/scripts/advanced/http-multipart.ts \
	examples/scripts/advanced/smtp-pipeline.ts \
	examples/scripts/advanced/tcp-proxy.ts \
	examples/scripts/advanced/https-server.ts \
	examples/scripts/advanced/sqlite-etl.ts \
	examples/scripts/advanced/crypto-pipeline.ts \
	examples/scripts/advanced/codec-interop.ts \
	examples/scripts/advanced/packet-analysis.ts \
	examples/scripts/advanced/recon-host-report.ts \
	examples/scripts/advanced/webdriver-login-flow.ts \
	examples/scripts/advanced/webdriver-actions.ts \
	examples/scripts/advanced/webdriver-grid.ts \
	examples/scripts/advanced/sqlite-migration.ts \
	examples/scripts/advanced/sse-stream.ts \
	examples/scripts/web-feed.ts \
	examples/scripts/web-sitemap.ts \
	examples/scripts/web-html.ts \
	examples/scripts/sheet.ts \
	examples/scripts/sheet-legacy.ts \
	examples/scripts/doc.ts \
	examples/recipes/sales-report.ts \
	examples/recipes/config-read.ts \
	examples/recipes/image-pipeline.ts \
	examples/recipes/format-convert.ts \
	examples/recipes/inventory.ts \
	examples/recipes/log-scan.ts \
	examples/recipes/stego-hide.ts \
	examples/recipes/stego-capacity.ts \
	examples/recipes/stego-detect.ts \
	examples/recipes/sheet-legacy-convert.ts \
	examples/recipes/xlsx-workbook.ts \
	examples/recipes/barcode-batch.ts \
	examples/recipes/doc-extract.ts

build:
	CGO_ENABLED=0 $(GO) build -o $(BIN) ./cmd/sercon

release:
	CGO_ENABLED=0 $(GO) build $(RELEASE_FLAGS) -o $(BIN) ./cmd/sercon
	@ls -lh $(BIN) | awk '{print "  built:", $$NF, "(" $$5 ")"}'

reference: build
	@./$(BIN) --emit-reference .manual-reference.tmp
	@awk '/<!-- BEGIN GENERATED REFERENCE -->/{print; while ((getline line < ".manual-reference.tmp") > 0) print line; skip=1; next} /<!-- END GENERATED REFERENCE -->/{skip=0} !skip{print}' MANUAL.md > MANUAL.md.ref.tmp
	@mv MANUAL.md.ref.tmp MANUAL.md
	@rm -f .manual-reference.tmp
	@echo "Regenerated the MANUAL.md '## 17. Binding reference' section."

manual: reference
	awk -f scripts/typst-safe.awk MANUAL.md | $(RECON) --md-to-pdf - -o MANUAL.pdf \
		--gfm --page-break-on-h1 \
		--font "IBM Plex Sans" \
		--cover \
		--toc --toc-depth 6 --toc-plain --toc-title "Contents" \
		--doc-title "sercon User Manual" \
		--doc-subtitle "Embeddable TypeScript script engine — reference and guide" \
		--doc-version "$(MANUAL_VERSION)" \
		--doc-date "$(MANUAL_DATE)" \
		--doc-author "Thomas Björk" \
		--doc-keywords "sercon, typescript, scripting, goja, esbuild, embedded"
# Manual is rendered by recon's typst engine: page-number footer, a page-numbered
# table of contents (--toc), a flag-generated cover, and a sans-serif body via
# recon-bundled IBM Plex Sans (no vendored font). MANUAL.md is piped through
# scripts/typst-safe.awk first (escapes prose angle brackets + strips HTML
# comments outside code) since the typst engine rejects raw HTML; the source file
# is left untouched so make reference / version-check / release-prep still work.
# The raw-HTML cover and hand-curated TOC were removed from MANUAL.md (the cover
# is now built from the --doc-* flags; the TOC from --toc).

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# Format the tree in place.
fmt:
	gofmt -w .

# Fail if any file needs gofmt (the CI gofmt gate; run `make fmt` to fix).
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt (run 'make fmt'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./... ; \
	else \
		echo "golangci-lint not installed; falling back to one-shot go run @$(GOLANGCI_VERSION)"; \
		$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./... ; \
	fi

demo: build
	@# -timeout 90s gives headroom above ai.ts's 60s inner send timeout: the
	@# AI CLI (claude -p, …) can run slow when another session is active, and
	@# the per-script engine timeout is the hard ceiling — at the 10s default
	@# the run is killed before ai.ts's own timeout + try/catch can degrade
	@# gracefully. Every other demo finishes in milliseconds, so the larger
	@# ceiling only matters for ai.ts.
	@./$(BIN) --verbose -timeout 90s $(DEMO_SCRIPTS)
	@echo "All example scripts passed. (hang.ts is the timeout demo — run separately.)"

# Opt-in integration tests against the dbplayground fleet
# (github.com/codedeviate/dbplayground). Brings the fleet up, runs the
# `Integration` tests with the SERCON_TEST_* vars pointed at it, then tears it
# down (-v) on exit — even on test failure. Requires Docker + Compose v2.
# Pin a fleet version with DBP_TAG (default latest).
test-integration:
	@command -v docker >/dev/null 2>&1 || { echo "test-integration: docker is required"; exit 1; }
	@set -e; \
	tmp=$$(mktemp -d); \
	trap 'docker compose -f $$tmp/dc.yml --profile ldap down -v >/dev/null 2>&1 || true; rm -rf $$tmp' EXIT; \
	curl -fsSL -o $$tmp/dc.yml https://raw.githubusercontent.com/codedeviate/dbplayground/main/docker-compose.hub.yml; \
	docker compose -f $$tmp/dc.yml --profile ldap up -d --wait; \
	SERCON_TEST_PG_DSN="postgresql://playground:playground@127.0.0.1:15432/testdb?sslmode=disable" \
	SERCON_TEST_MYSQL_DSN="playground:playground@tcp(127.0.0.1:13306)/testdb" \
	SERCON_TEST_MARIADB_DSN="playground:playground@tcp(127.0.0.1:13307)/testdb" \
	SERCON_TEST_CLICKHOUSE_DSN="clickhouse://playground:playground@127.0.0.1:19000/testdb" \
	SERCON_TEST_REDIS_URL="redis://127.0.0.1:16379/0" \
	SERCON_TEST_VALKEY_URL="valkey://127.0.0.1:16380/0" \
	SERCON_TEST_MEMCACHED_ADDR="127.0.0.1:13211" \
	SERCON_TEST_LDAP_URL="ldap://127.0.0.1:13389" \
	SERCON_TEST_LDAP_BINDDN="cn=admin,dc=example,dc=org" \
	SERCON_TEST_LDAP_PASSWORD="adminpw" \
	SERCON_TEST_LDAP_BASE="dc=example,dc=org" \
	$(GO) test ./cmd/sercon/ -run '^TestIntegration_' -v -count=1

types: build
	./$(BIN) --emit-dts examples/scripts/sercon.d.ts

release-prep:
	@if [ -z "$(VERSION)" ]; then \
		echo "usage: make release-prep VERSION=x.y.z"; exit 2; \
	fi
	@echo "Bumping version markers to $(VERSION)..."
	@sed -i.bak -E 's/(^const Version = ")[^"]+(")/\1$(VERSION)\2/' pkg/scriptengine/version.go
	@sed -i.bak -E 's/(\*This manual covers sercon v)[0-9.]+(\.)/\1$(VERSION)\2/' MANUAL.md
	@# The PDF cover (title/version/date) is generated by the typst engine from the
	@# make manual --doc-* flags (version sourced from version.go, date stamped at
	@# render), so there is no in-MANUAL cover marker to bump — only the footer.
	@# HISTORY.md "covered … through vX.Y.Z (YYYY-MM-DD)" span line — bumped here
	@# so it ships in the cut commit (otherwise the span bump perpetually trails a
	@# release). Capability narrative is still added per-feature by hand.
	@d=$$(date +%Y-%m-%d); sed -i.bak -E "s|(through v)[0-9]+\.[0-9]+\.[0-9]+ \([0-9-]+\)|\1$(VERSION) ($$d)|" HISTORY.md
	@rm -f pkg/scriptengine/version.go.bak MANUAL.md.bak HISTORY.md.bak
	@$(MAKE) --no-print-directory version-check
	@echo ""
	@echo "Next steps:"
	@echo "  (version.go, MANUAL.md strings, and the HISTORY.md span line are bumped already.)"
	@echo "  1) Edit CHANGELOG.md: move the [Unreleased] entries into [$(VERSION)] - $$(date +%Y-%m-%d)"
	@echo "  2) make manual && make types && make test && make vet && make lint && make demo"
	@echo "  3) git commit -am 'chore: cut v$(VERSION)'"
	@echo "  4) git tag -a v$(VERSION) -m 'release v$(VERSION)'"
	@echo "  5) git push origin master v$(VERSION)  # CI publishes binaries via goreleaser"
	@echo "  6) make release-verify VERSION=$(VERSION)  # wait for the goreleaser GitHub release to publish"
	@echo "  7) make homebrew-bump VERSION=$(VERSION)  # bump + push the Homebrew tap formula"

# release-verify VERSION=x.y.z
#   Poll the GitHub release for vX.Y.Z until goreleaser has published it (not a
#   draft, with its checksums.txt asset uploaded). Run AFTER pushing the tag and
#   BEFORE homebrew-bump, so the binary release is confirmed live (and the
#   goreleaser job didn't fail) before the tap is bumped. Requires gh
#   authenticated for RELEASE_REPO. ~10 min cap (60 x 10s).
release-verify:
	@if [ -z "$(VERSION)" ]; then echo "usage: make release-verify VERSION=x.y.z"; exit 2; fi
	@echo "Waiting for the goreleaser release v$(VERSION) on $(RELEASE_REPO)..."
	@for i in $$(seq 1 60); do \
		ready=$$(gh release view "v$(VERSION)" --repo "$(RELEASE_REPO)" --json assets,isDraft \
			--jq '((.isDraft | not) and ([.assets[].name] | index("checksums.txt") | type == "number"))' 2>/dev/null); \
		if [ "$$ready" = "true" ]; then \
			n=$$(gh release view "v$(VERSION)" --repo "$(RELEASE_REPO)" --json assets --jq '.assets | length' 2>/dev/null); \
			echo "  release v$(VERSION) is live ($$n assets, checksums.txt present)."; \
			exit 0; \
		fi; \
		echo "  not ready yet; retrying in 10s ($$i/60)..."; \
		sleep 10; \
	done; \
	echo "ERROR: release v$(VERSION) not published with assets after ~10 min."; \
	echo "  The goreleaser job may have failed — check: gh run list --workflow=release.yml"; \
	exit 1

# Bump the Homebrew formula in the codedeviate/homebrew-cli tap and push it.
# Reuses the tap's own scripts/bump.sh (which fetches the release tarball to
# recompute sha256), so the vX.Y.Z tag must already be pushed and the repo
# public. The tap's origin uses an SSH host alias (github-codedv8) that carries
# the codedeviate identity, so the push needs no gh-account switch. Idempotent:
# re-running for an already-bumped version makes no commit.
homebrew-bump:
	@if [ -z "$(VERSION)" ]; then \
		echo "usage: make homebrew-bump VERSION=x.y.z"; exit 2; \
	fi
	@if [ ! -x "$(HOMEBREW_TAP)/scripts/bump.sh" ]; then \
		echo "error: $(HOMEBREW_TAP)/scripts/bump.sh not found (set HOMEBREW_TAP=/path/to/homebrew-cli)"; exit 1; \
	fi
	bash "$(HOMEBREW_TAP)/scripts/bump.sh" sercon "$(VERSION)"
	@if git -C "$(HOMEBREW_TAP)" diff --quiet -- Formula/sercon.rb; then \
		echo "Formula already at $(VERSION); nothing to commit."; \
	else \
		git -C "$(HOMEBREW_TAP)" add Formula/sercon.rb; \
		git -C "$(HOMEBREW_TAP)" commit -m "sercon $(VERSION)"; \
		git -C "$(HOMEBREW_TAP)" push; \
		echo "Pushed Homebrew formula bump: sercon $(VERSION)"; \
	fi

sample-data: build
	./$(BIN) examples/data/generate.ts

paymentproviders:        ## bundle the embedded paymentproviders TS library
	go run ./cmd/ppbundle

paymentproviders-check:  ## fail if the committed paymentproviders bundle is stale
	go run ./cmd/ppbundle --check

favro:                   ## bundle the embedded favro TS library
	go run ./cmd/favrobundle

favro-check:             ## fail if the committed favro bundle is stale
	go run ./cmd/favrobundle --check

version-check:
	@const=$$(sed -nE 's|^const Version = "([^"]+)".*$$|\1|p' pkg/scriptengine/version.go); \
	footer=$$(sed -nE 's|\*This manual covers sercon v([0-9.]+)\..*|\1|p' MANUAL.md); \
	if [ -z "$$const" ] || [ -z "$$footer" ]; then \
		echo "version markers not found: code='$$const' footer='$$footer'"; \
		exit 1; \
	fi; \
	if [ "$$const" != "$$footer" ]; then \
		echo "version mismatch: code=$$const footer=$$footer"; \
		exit 1; \
	fi; \
	echo "version markers in sync at $$const"
	@go run ./cmd/ppbundle --check
	@go run ./cmd/favrobundle --check

clean:
	rm -f $(BIN) MANUAL.pdf
