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

## Architecture

One binary, four layers, one direction of dependency — `cmd` → `internal` → nothing.
No layer reaches back up, so each can be read (and tested) on its own.

```
cmd/server        wiring only: config in, deps constructed, handler served (~50 lines)
cmd/ingest        the indexing CLI

internal/server   HTTP: routes, cache policy, SSE. Knows no SQLite and no templates.
internal/rag      the domain: chunk → embed → retrieve → grounded answer
internal/ai       one OpenAI-compatible client (embeddings + chat streaming)
internal/db       SQLite: sqlite-vec + FTS5, hybrid search with RRF
internal/config   env → Config, with defaults

web/              index.html (a Go template) + embed.go
web/app/          the app shell — native ES modules, no build step
web/vendor/       `make vendor` output (gitignored)
```

**Where do I add…?**

| …this | goes here | because |
|---|---|---|
| a new API endpoint | `internal/server` | routing and transport live in one place |
| better retrieval / prompting | `internal/rag` | the domain never imports HTTP |
| a new provider | `internal/ai` | one client, one seam |
| a UI action | `web/app/app.js` | it's intent; plumbing lives in its own module |
| a fetch/stream concern | `web/app/chat.js` | the app must not learn about SSE |
| markdown / citation rendering | `web/app/answer.js` | sanitising is one file's job |
| a mobile viewport quirk | `web/app/viewport.js` | keyboard/dock/scroll maths, hidden |
| a layout rule | `web/app/styles.css` | 8bit-nes owns components; this owns layout |

### The two seams that make it testable

`internal/server` depends on a one-method interface, not on the engine:

```go
type Answerer interface {
    Answer(ctx context.Context, question string, onToken func(string)) ([]rag.Citation, error)
}
```

So the whole HTTP surface — SSE framing, cache headers, 400s — is covered by
`go test` with a fake, needing no API key and no database. On the front end, the
same idea: `app.js` never sees an `AbortController`, a `TextDecoder`, a
`ResizeObserver` or `visualViewport`. It says

```js
const run = ask(question, { onToken, onCitations });   // chat.js
run.stop();                                            // resolves, never throws
view.scrollToEnd();                                    // viewport.js: follows, unless the reader scrolled up
```

`make check` runs the lot (`gofmt`, `go vet`, `go test`).

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
- **No auth.** Run it behind your VPN/LAN. Auth/RBAC is a later phase.

## Frontend assets (CDN, pinned)

The UI is one embedded HTML file that pulls four things from jsDelivr: Vue,
`marked`, DOMPurify, and the [8-BIT NES](https://github.com/TuTranMVP/8bit-components)
design system (**0.5.0**). All of it is version-pinned *and* hash-pinned:

| | pin |
|---|---|
| one origin | a single `preconnect`ed host → one DNS + TLS handshake for every asset |
| exact versions | `vue@3.5.40`, `marked@18.0.7`, `dompurify@3.4.12`, `8bit-nes@0.5.0` — a pinned jsDelivr URL is `immutable`, cached a year, and can't change under a deployed page |
| `integrity` | `sha384` on all four; the browser refuses a byte that doesn't match |
| `defer` + module | ~240 kB of `<script>` no longer blocks the parser; the app boots from the inline module, which runs after them by spec |
| font `preload` | the three woff2 faces start with the stylesheet, at the exact URLs the CSS resolves — each fetched once |

Digests live in [`web/vendor.sha384`](web/vendor.sha384) — the same values the
HTML carries. 8-BIT NES publishes its own at
[`/sri.json`](https://tutranmvp.github.io/8bit-components/sri.json).

> **Why pin so hard?** Because the floating spec broke this page: `marked` stopped
> shipping `marked.min.js` at its package root after v4, so the old unpinned
> `npm/marked/marked.min.js` began 404ing and answers silently stopped rendering.

### Air-gapped / no egress

Serve every asset from the binary instead:

```bash
make vendor      # fetch + sha384-verify into web/vendor/ (npm registry, not the CDN)
make build       # whatever is in web/vendor/ gets embedded
ASSET_BASE=/vendor ./bin/knowledge
```

`ASSET_BASE` is the only switch — the asset paths in `web/index.html` mirror the
npm layout, so the same URLs work against a CDN or against `/vendor` (served with
`Cache-Control: immutable`, since every path carries its version). Vendored files
are gitignored; run `make vendor` on a machine that *does* have egress, then ship
the binary. To upgrade a dependency, bump the version and digest in
`web/vendor.sha384` and `web/index.html`, then re-run `make vendor`.
```
