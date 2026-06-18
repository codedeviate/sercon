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
#            Bump every version marker (Go const + MANUAL cover + footer)
#            in one shot, then print the next-step checklist. Releases are
#            cut manually: this + edit CHANGELOG + make manual + tag + push.
#            See CLAUDE.md > Versioning and commits.
#   version-check
#            Verify the version markers in pkg/scriptengine/version.go and
#            MANUAL.md all agree. Run by release-prep; useful standalone
#            after editing one of the two by hand.
#   clean    Remove built artifacts
#
# release and manual are intentionally separate from `build` so an
# interactive dev cycle doesn't pay their costs.

GO                ?= go
RECON             ?= recon
GOLANGCI_VERSION  ?= v2.12.2
BIN                = sercon
RELEASE_FLAGS      = -trimpath -ldflags=-s\ -w

.PHONY: build release manual reference test test-integration vet lint demo types release-prep version-check clean

DEMO_SCRIPTS = \
	examples/scripts/smoke.ts \
	examples/scripts/async.ts \
	examples/scripts/argv.ts \
	examples/scripts/console.ts \
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
	examples/scripts/dump-codec.ts \
	examples/scripts/codec-xml.ts \
	examples/scripts/archive.ts \
	examples/scripts/diff.ts \
	examples/scripts/jq.ts \
	examples/scripts/exec-shell.ts \
	examples/scripts/exec-stream.ts \
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
	examples/scripts/net-sockets.ts \
	examples/scripts/load.ts \
	examples/scripts/capture-file.ts \
	examples/scripts/image.ts \
	examples/scripts/tui-keys.ts \
	examples/scripts/agent-browser-core.ts \
	examples/scripts/agent-browser-capture.ts \
	examples/scripts/agent-browser-state.ts \
	examples/scripts/agent-browser-advanced.ts \
	examples/scripts/webdriver.ts \
	examples/scripts/webdriver-advanced.ts \
	examples/scripts/typst.ts \
	examples/scripts/advanced/load-resilience.ts \
	examples/scripts/advanced/http-api.ts \
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
	examples/scripts/advanced/sse-stream.ts

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
	@echo "Regenerated the MANUAL.md '## 16. Binding reference' section."

manual: reference
	$(RECON) --md-to-pdf MANUAL.md -o MANUAL.pdf \
		--pdf-engine chrome \
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
	@# -timeout 90s gives headroom above ai.ts's 60s inner send timeout: the
	@# AI CLI (claude -p, …) can run slow when another session is active, and
	@# the per-script engine timeout is the hard ceiling — at the 10s default
	@# the run is killed before ai.ts's own timeout + try/catch can degrade
	@# gracefully. Every other demo finishes in milliseconds, so the larger
	@# ceiling only matters for ai.ts.
	@./$(BIN) -timeout 90s $(DEMO_SCRIPTS)
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
	@sed -i.bak -E 's|(<div class="version">Version )[^<]+(</div>)|\1$(VERSION)\2|' MANUAL.md
	@sed -i.bak -E 's/(\*This manual covers sercon v)[0-9.]+(\.)/\1$(VERSION)\2/' MANUAL.md
	@d=$$(date +%Y-%m-%d); sed -i.bak -E "s|(<div class=\"date\">)[^<]+(</div>)|\1$$d\2|" MANUAL.md
	@rm -f pkg/scriptengine/version.go.bak MANUAL.md.bak
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
	if [ -z "$$const" ] || [ -z "$$cover" ] || [ -z "$$footer" ]; then \
		echo "version markers not found: code='$$const' cover='$$cover' footer='$$footer'"; \
		exit 1; \
	fi; \
	if [ "$$const" != "$$cover" ] || [ "$$const" != "$$footer" ]; then \
		echo "version mismatch: code=$$const cover=$$cover footer=$$footer"; \
		exit 1; \
	fi; \
	coverdate=$$(sed -nE 's|.*<div class="date">([0-9]{4}-[0-9]{2}-[0-9]{2})</div>.*|\1|p' MANUAL.md); \
	logdate=$$(sed -nE "s|^## \[$$const\][^0-9]*([0-9]{4}-[0-9]{2}-[0-9]{2}).*|\1|p" CHANGELOG.md | head -1); \
	if [ -n "$$logdate" ] && [ "$$coverdate" != "$$logdate" ]; then \
		echo "date mismatch: manual cover date $$coverdate != CHANGELOG [$$const] date $$logdate"; \
		exit 1; \
	fi; \
	echo "version markers in sync at $$const ($${coverdate:-no cover date})"

clean:
	rm -f $(BIN) MANUAL.pdf
