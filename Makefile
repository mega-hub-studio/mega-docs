# sqlite-vec (cgo) + FTS5 require CGO + these build tags.
TAGS := sqlite_fts5
export CGO_ENABLED := 1

.PHONY: deps check test secrets live smoke server ingest build vendor vendor-clean clean

deps:
	go mod tidy

# Everything CI should gate on: formatting, vet, tests.
check: test secrets
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed in:"; gofmt -l .; exit 1; }
	go vet -tags "$(TAGS)" ./...

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

clean:
	rm -rf bin knowledge.db knowledge.db-*
