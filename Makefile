# sqlite-vec (cgo) + FTS5 require CGO + these build tags.
TAGS := sqlite_fts5
export CGO_ENABLED := 1

.PHONY: deps check test dead secrets live smoke server ingest build vendor vendor-clean diagram clean

deps:
	go mod tidy

# Everything CI should gate on: formatting, vet, tests.
check: test secrets
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed in:"; gofmt -l .; exit 1; }
	go vet -tags "$(TAGS)" ./...
	@$(MAKE) --no-print-directory dead

# Dead code has a way of accumulating quietly. staticcheck catches unused
# declarations; deadcode catches functions no binary can reach. Missing tools are
# reported as skipped — but a *finding* must fail the build, so neither is written
# as `tool || echo skipped` (that turns a non-zero exit into a cheerful message).
# deadcode only reports, so its output is what decides the exit code.
dead:
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck -tags "$(TAGS)" ./...; \
	else echo "  skipped staticcheck (go install honnef.co/go/tools/cmd/staticcheck@latest)"; fi
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
