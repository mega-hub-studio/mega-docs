# sqlite-vec (cgo) + FTS5 require CGO + these build tags.
TAGS := sqlite_fts5
export CGO_ENABLED := 1

# The Go packages, spelled out rather than `./...`.
#
# web/ui is a Vite project, and npm installs whatever its dependencies ship — one of them
# (flatted) ships a Go package. `./...` walks into web/ui/node_modules and hands the
# linter somebody else's code: seven findings that are not this repository's, in a gate
# whose whole value is that a finding means something. The Go tool has no ignore for
# directories, so the pattern is the fix.
PKGS := ./cmd/... ./internal/... ./web

.PHONY: deps check check-full ui-deps test lint lint-fix lint-js dead secrets live smoke server ingest build switch-embed vendor vendor-clean diagram clean check-ui check-wt ui ui-dev

deps:
	go mod tidy

# Everything CI should gate on: formatting, vet, tests, linters.
check: test secrets
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed in:"; gofmt -l .; exit 1; }
	go vet -tags "$(TAGS)" $(PKGS)
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory lint-js
	@$(MAKE) --no-print-directory dead

# ── THE FINAL GATE: one command to run after implementing anything ──
# `check` is what CI gates on and what you run while working. This is the superset you
# run *before* saying it is done, and the order is the point: each stage is cheaper than
# the next, so the first thing to break is the first thing you hear about.
#
#   1. ui        rebuild the bundle FIRST — TestBuiltUIMatchesItsSources (inside `check`)
#                compares web/dist against a hash of its sources, so running `check`
#                before this reports a stale bundle rather than the bug you introduced.
#   2. check     gofmt · vet · every Go test · golangci-lint · deadcode · credential scan
#                · eslint (which is also the formatter now: style rules are errors, so a
#                missing reformat fails here rather than in review)
#   3. build     both binaries, because `go build` catches what `go vet` does not
#   4. check-ui  the guide rendered, served and measured in Chromium, both languages
#   5. check-wt  every diagram walkthrough driven prev/next at two viewports
#
# 4 and 5 need node + playwright and *skip* (0) without them, so this target stays
# runnable on a box that has neither — read the "skipped" lines rather than assuming a
# green run covered them. Same for deadcode inside `check`.
check-full:
	@$(MAKE) --no-print-directory ui
	@$(MAKE) --no-print-directory check
	@$(MAKE) --no-print-directory build
	@$(MAKE) --no-print-directory check-ui
	@$(MAKE) --no-print-directory check-wt
	@echo ""
	@echo "  check-full: PASS — bundle fresh, Go + JS clean, guide and walkthroughs measured"

# golangci-lint, configured by .golangci.yml — which explains every linter it turns off,
# because the stock config reports 591 issues on this tree and a gate that always shouts
# is a gate nobody reads. It currently reports zero; a new finding means a new fact.
# staticcheck runs inside it, which is why `dead` no longer runs it separately.
# Prints the version it used. CI installs @latest, so an older binary left on PATH
# locally passes what CI then fails — that happened, on goconst and a new gosec rule,
# after the gate had already gone green here.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint version | head -1; \
		golangci-lint run $(PKGS); \
	else echo "  skipped golangci-lint (go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)"; fi

# ── the app's front end ───────────────────────────────────────────────────────
# web/ui is a Vite project (Vue 3.5 SFCs, JavaScript). Its output, web/dist, is committed
# and embedded by the binary — so `go build`, `go install` and `git pull && make build` on
# the host all keep working without Node. That is the whole reason a build artefact is in
# git, and TestBuiltUIMatchesItsSources is what stops it going stale.
# node_modules has to match the lockfile, and "it exists" is not the same question.
# `[ -d node_modules ] || npm ci` was the old test, so a `git pull` that *added* a
# dependency left a directory that existed and was stale — and the failure surfaced as a
# Vite overlay saying `Failed to resolve import "vue-i18n" from "src/main.js"`, pointing at
# your own source, which is the least useful place to look. That happened, on the commit
# that added vue-i18n.
#
# npm writes node_modules/.package-lock.json on every install, so a lockfile newer than that
# stamp means the tree is behind. Installing is the right move rather than only reporting:
# nobody wants "run npm ci" from a target whose whole job is to build the front end.
UI_STAMP := web/ui/node_modules/.package-lock.json
ui-deps:
	@if [ ! -f $(UI_STAMP) ] || [ web/ui/package-lock.json -nt $(UI_STAMP) ]; then \
		echo "  web/ui/node_modules is missing or behind package-lock.json — installing"; \
		cd web/ui && npm ci --no-audit --no-fund; \
	fi

ui: ui-deps
	@cd web/ui && npm run build

# HMR against the real engine: serves the UI on :5173 and proxies /api to :8080, so run
# `make server` in another shell first.
ui-dev: ui-deps
	@cd web/ui && npm run dev

# The walkthroughs, driven. Separate from check-ui because it answers a different
# question: check-ui measures layout, this one measures behaviour — prev/next, the dots,
# the keyboard, and whether each step's `data-focus` still matches a node in its diagram.
# A renamed node lights nothing, which no layout measurement would notice.
check-wt:
	@./scripts/check-walkthroughs.sh

# The browser check: what a screenshot shows, measured. Not in `check` — it needs a
# browser, and this product needs no Node at all (see CLAUDE.md rule 14). Run it after
# touching a docs page, a recipe or docsbase.html.
check-ui:
	@./scripts/check-docs-ui.sh

# ESLint over web/ui — and it is in `check`, unlike before. What changed is that there are
# .vue files now: eslint-plugin-vue's `vue/no-undef-properties` reads a real parse of a
# real component and catches a template binding with nothing behind it, which is the rule a
# Go regex used to approximate. It runs from the project's own devDependencies (installed
# by `make ui`), so there is no second eslint cache to keep in step.
#
# Skipped, loudly, when node_modules is absent: a machine that only builds Go still has a
# committed bundle and does not need a linter for it.
lint-js:
	@[ -d web/ui/node_modules ] || { echo "  skipped lint-js (run \`make ui\` to install web/ui)"; exit 0; }
	@cd web/ui && npm run --silent lint

# Same linters, applying the fixes they know how to make. Read the diff: the formatters
# are opinionated and one of them (gofumpt) is turned off here for a reason .golangci.yml
# spells out.
lint-fix:
	golangci-lint run --fix $(PKGS)

# What no linter finds: a function no binary can reach. staticcheck's unused only sees
# within a package, and it now runs inside `lint` anyway; deadcode does whole-program
# reachability from the two mains. A missing tool is reported as skipped — but a
# *finding* must fail the build, which is why this is not written as `tool || echo`
# (that turns a non-zero exit into a cheerful message).
dead:
	@if command -v deadcode >/dev/null 2>&1; then \
		out=$$(deadcode -tags "$(TAGS)" ./cmd/...); \
		if [ -n "$$out" ]; then echo "$$out"; echo "^ unreachable from any binary"; exit 1; fi; \
	else echo "  skipped deadcode (go install golang.org/x/tools/cmd/deadcode@latest)"; fi

# Nothing key-shaped may be committed. .env is gitignored; this catches the case
# where a key gets pasted into a tracked file by accident.
# Three of the exclusions are generated files whose content is hashes by definition:
# web/dist (Vite's bundle, where minified third-party code matches by accident) and BOTH
# lockfiles (every entry carries a sha512 integrity string). Excluding the *build output* is
# not a blind spot — a key can only reach the bundle from web/ui/src, which is scanned.
#
# pnpm-lock.yaml is here because it failed exactly the way package-lock.json did, two
# minutes after it was committed: same integrity strings, same red gate. That two lockfiles
# exist at all is a separate problem — CI installs with `npm ci`, so package-lock.json is
# what it builds the committed bundle from, and a pnpm tree that resolves anything
# differently makes the rule-14 diff compare a bundle built from other dependencies. Pick
# one manager; until then this only stops the scan from crying wolf.
#
# git grep only sees TRACKED files, which is worth knowing before trusting a green run:
# running this before `git add` scans a smaller tree than CI does. That is exactly how the
# lockfile got past a local `make check` and failed on the first push.
secrets:
	@! git grep -nIE '(sk|api|key|token)[-_]?[A-Za-z0-9]{24,}' -- . \
		':!*.sha384' ':!scripts/*' ':!web/dist/*' \
		':!web/ui/package-lock.json' ':!web/ui/pnpm-lock.yaml' \
		|| { echo "^ that looks like a credential in a tracked file"; exit 1; }

test:
	go test -tags "$(TAGS)" $(PKGS)

# Run the chat server (http://localhost:8080)
server:
	go run -tags "$(TAGS)" ./cmd/server

# Index docs:  make ingest DOCS=./docs
ingest:
	go run -tags "$(TAGS)" ./cmd/ingest $(DOCS)

# Compile a single self-contained binary. Whatever is in web/vendor/ at this
# point gets embedded, so `make vendor build` produces an egress-free binary.
build:
	go build -tags "$(TAGS)" -o bin/knowledge ./cmd/server
	go build -tags "$(TAGS)" -o bin/ingest   ./cmd/ingest

# Probe a real provider: does it have both endpoints, what embedding width, does
# chat stream? Skipped unless AI_API_KEY is set (read from .env).
live:
	go test -tags "$(TAGS) live" -v -count=1 -run TestLive ./internal/ai/

# Full round trip against a real provider: ingest a fixture, ask, verify the
# answer streams and cites it.
smoke:
	sh scripts/smoke.sh

# Point embeddings at another provider: validate the key, then re-index (vectors
# from two models are not comparable). Edit .env first — it is the source of truth.
#   DIR=/opt/knowledge SERVICE=knowledge make switch-embed
switch-embed:
	sh scripts/switch-embed.sh

# Download + digest-verify the front-end's CDN assets into web/vendor/, so the
# binary can serve them itself (ASSET_BASE=/vendor). Pins live in web/vendor.sha384.
vendor:
	sh scripts/vendor.sh

vendor-clean:
	find web/vendor -mindepth 1 ! -name .gitkeep -delete

# Render web/*.mmd to web/*.svg. Only needed after editing a diagram — the SVG is
# committed, so a normal build and CI never run this and never need mermaid.
# Requires the vendored assets (for the design system's own theme + fonts) and
# fetches mermaid into .cache/ as a build-time-only tool.
diagram: vendor
	node scripts/gen-diagram.mjs

clean:
	rm -rf bin knowledge.db knowledge.db-*
