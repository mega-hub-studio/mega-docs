# sqlite-vec (cgo) + FTS5 require CGO + these build tags.
TAGS := sqlite_fts5
export CGO_ENABLED := 1

.PHONY: deps server ingest build vendor vendor-clean clean

deps:
	go mod tidy

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

# Download + digest-verify the front-end's CDN assets into web/vendor/, so the
# binary can serve them itself (ASSET_BASE=/vendor). Pins live in web/vendor.sha384.
vendor:
	sh scripts/vendor.sh

vendor-clean:
	find web/vendor -mindepth 1 ! -name .gitkeep -delete

clean:
	rm -rf bin knowledge.db knowledge.db-*
