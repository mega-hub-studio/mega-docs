# sqlite-vec (cgo) + FTS5 require CGO + these build tags.
TAGS := sqlite_fts5
export CGO_ENABLED := 1

.PHONY: deps check test lint lint-fix lint-js dead secrets live smoke server ingest build switch-embed vendor vendor-clean diagram clean

deps:
	go mod tidy

# Everything CI should gate on: formatting, vet, tests, linters.
check: test secrets
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed in:"; gofmt -l .; exit 1; }
	go vet -tags "$(TAGS)" ./...
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory dead

# golangci-lint, configured by .golangci.yml — which explains every linter it turns off,
# because the stock config reports 591 issues on this tree and a gate that always shouts
# is a gate nobody reads. It currently reports zero; a new finding means a new fact.
# staticcheck runs inside it, which is why `dead` no longer runs it separately.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else echo "  skipped golangci-lint (go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)"; fi

# The JavaScript net, on demand and with nothing committed for it.
#
# eslint + @antfu/eslint-config are installed into .cache/ — the same throwaway tool cache
# `make diagram` uses for mermaid — because they are 255 packages the product does not
# need, and this repo has no package.json for exactly that reason. The config itself is
# tracked (eslint.config.mjs) and explains what it turns off.
#
# Not part of `make check`: the architecture rules that catch this codebase's real mistakes
# are in web/frontend_test.go and cost nothing. See the note at the top of eslint.config.mjs.
JS_CACHE := .cache/eslint
lint-js:
	@command -v npm >/dev/null 2>&1 || { echo "  skipped lint-js (needs npm; the product does not)"; exit 0; }
	@mkdir -p $(JS_CACHE)
	@cp eslint.config.mjs $(JS_CACHE)/
	@[ -d $(JS_CACHE)/node_modules ] || { \
		printf '{ "name":"eslint-cache","private":true,"type":"module" }\n' > $(JS_CACHE)/package.json; \
		echo "  installing eslint into $(JS_CACHE) (once)"; \
		(cd $(JS_CACHE) && npm i -D --silent eslint@9 @antfu/eslint-config@latest); }
	$(JS_CACHE)/node_modules/.bin/eslint --config $(JS_CACHE)/eslint.config.mjs web/app

# Same linters, applying the fixes they know how to make. Read the diff: the formatters
# are opinionated and one of them (gofumpt) is turned off here for a reason .golangci.yml
# spells out.
lint-fix:
	golangci-lint run --fix ./...

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
secrets:
	@! git grep -nIE '(sk|api|key|token)[-_]?[A-Za-z0-9]{24,}' -- . ':!*.sha384' ':!scripts/*' \
		|| { echo "^ that looks like a credential in a tracked file"; exit 1; }

test:
	go test -tags "$(TAGS)" ./...

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
