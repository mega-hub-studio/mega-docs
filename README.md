# Knowledge Engine — MVP

Self-hosted RAG for internal technical/business docs. One Go binary + one SQLite
file. Semantic + keyword hybrid search, grounded answers, citations.

```
[md/txt docs] → ingest (chunk → embed → SQLite) 
                                   │
[Vue chat] ──SSE──> /api/chat ──hybrid search (vec + BM25, RRF)──┘
                        └─> LLM (OpenAI-compatible) → grounded stream + citations
```

Everything ships in the `knowledge` binary (Vue UI is embedded via `go:embed`).
No Node, no Docker, no external vector DB.

## Prerequisites

- Go 1.22.5+
- A C compiler (gcc/clang) + `sqlite3.h` (Debian/Ubuntu: `apt install libsqlite3-dev`;
  macOS: preinstalled with Xcode CLT) — required by the cgo SQLite bindings.
- An API key from any OpenAI-compatible provider (or run Ollama locally, free).

## Quick start

```bash
cp .env.example .env      # then edit AI_API_KEY (and AI_BASE_URL if not OpenAI)
make deps                 # go mod tidy

# 1) Index your docs (folder or files; .md/.txt only)
make ingest DOCS=./docs

# 2) Start the chat server
make server               # http://localhost:8080
```

Ship it as a single binary instead:

```bash
make build && ./bin/knowledge
```

## Ingesting PDF / DOCX

Go is weak at parsing binary docs — don't fight it. Convert to clean markdown
first, then ingest the markdown:

```bash
pip install markitdown
markitdown spec.pdf > docs/spec.md
make ingest DOCS=./docs
```

`MarkItDown` (Microsoft) or `Docling` (IBM) both work well. This keeps the Go
binary clean and makes "docs are messy/noisy" a one-time cleaning step, not a
runtime problem.

## How answers stay grounded

- Retrieval fuses vector similarity (`sqlite-vec`) and BM25 keyword match
  (`FTS5`) with Reciprocal Rank Fusion. Keyword match is what catches function
  names, error codes and config keys that pure semantic search misses.
- The LLM is instructed to answer **only** from retrieved context, to say so
  when the answer isn't there, and to cite every claim `[n]`.

## Phase-1 hooks already in place

The schema carries `status (draft|approved)` and `version` on every chunk, and
retrieval already boosts `approved` chunks. Wiring up a BA approval UI later is
additive — no migration, no re-architecture.

## Caveats (read before first build)

- **sqlite-vec Go bindings evolve.** If `go mod tidy` or the build complains
  about the `github.com/asg017/sqlite-vec-go-bindings` version or the
  `sqlite_vec.Auto()` / `SerializeFloat32` API, check the current README for
  that module and adjust the version in `go.mod` / the calls in
  `internal/db/store.go`. This scaffold was not compiled in this environment.
- **FTS5 build tag.** Uses `-tags sqlite_fts5`. If your `go-sqlite3` version
  wants `-tags fts5` instead, change it in the `Makefile`.
- **Frontend uses CDN** (Vue/marked/DOMPurify). For an offline internal network,
  download those three files into `web/` and switch the `<script src>` paths to
  local — the embed picks them up automatically.
- **No auth.** Run it behind your VPN/LAN. Auth/RBAC is a later phase.
```
