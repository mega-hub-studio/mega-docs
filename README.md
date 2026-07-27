# Knowledge Engine — MVP

Self-hosted RAG for internal technical/business docs. One Go binary + one SQLite
file. Semantic + keyword hybrid search, grounded answers, citations.

```
[md/txt docs] → ingest (chunk → embed → SQLite)
                                   │
[Vue chat] ──SSE──> /api/chat ──hybrid search (vec + BM25, RRF)──┘
                        ├─> answer cache hit → free, no provider call
                        └─> LLM (OpenAI-compatible) → grounded stream + citations

no answer? → DEV files a ticket → BA confirms → docs/qa/ticket-N.md → indexed
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

The binary *is* the web server — the UI is embedded, so there is no frontend to
deploy separately.

The bilingual (EN/VI) guide is published as static pages on its own domain. One page
per role, because each role arrives with a different question:

| | Guide | Dev | Deploy |
|---|---|---|---|
| Answers | "what is this, can I trust this answer?" | "where do I change X?" | "how do we run it for the team?" |
| For | everyone, then BA / PM / support | DEV | whoever hosts it |
| Published at | [/](https://mega-hub-studio.github.io/mega-docs/) | [/dev.html](https://mega-hub-studio.github.io/mega-docs/dev.html) | [/deploy.html](https://mega-hub-studio.github.io/mega-docs/deploy.html) |

The Guide opens with a router, so nobody has to read a page that is not theirs.

For AI agents there is [`AGENTS.md`](AGENTS.md) and a generated
[`/llms.txt`](https://mega-hub-studio.github.io/mega-docs/llms.txt) — an
[llmstxt.org](https://llmstxt.org) index of every page and section, built by
`cmd/rendocs` from the pages themselves so it cannot drift from them. `AGENTS.md`
also points at the **version-pinned** 8bit-nes docs (`llms.txt` ships inside the
package, so a jsDelivr URL is version-exact, unlike the always-latest docs site) and
a test fails if that version drifts from `web/vendor.sha384`.

**Guide** is what it is, how it works, three steps to running it, using it well
(asking, reading citations, what is actually indexed) and the four failures that
actually happen. **Dev** is the first hour: where things live, the two seams,
testing with no API key, and the knobs people actually turn — it points here for
the full reference rather than restating it. **Deploy** is letting the team in
(Tailscale / Cloudflare Tunnel / LAN), where to run it, systemd, backups and the
settings table.

The app binary does **not** serve the guide. Documentation has its own domain, and a
second copy inside the app is noise on the one surface people came to use — plus a
copy to drift. The app carries a single link out to the published guide. Build the
pages locally with `go run ./cmd/rendocs -d /tmp/site -base /vendor`.

*Live, and published by CI on every push to `main` that touches the guide. If you
fork this: turn Pages on once under Settings → Pages → Source **GitHub Actions**. A
workflow token cannot do it for you — creating a Pages site needs repo-admin rights —
and until it is on, every run fails at `configure-pages`.*

## Architecture

One binary, four layers, one direction of dependency — `cmd` → `internal` → nothing.
No layer reaches back up, so each can be read (and tested) on its own.

```
cmd/server        wiring only: config in, deps constructed, handler served (~50 lines)
cmd/ingest        the indexing CLI
cmd/rendocs       renders every guide page + llms.txt to static files; no cgo

internal/server   HTTP: routes, cache policy, SSE. Knows no SQLite and no templates.
internal/rag      the domain: chunk → embed → retrieve → grounded answer
internal/ai       one OpenAI-compatible client (embeddings + chat streaming)
internal/aitest   a fake provider over httptest — the whole pipeline, no key needed
internal/db       SQLite: sqlite-vec + FTS5, hybrid search with RRF,
                  tickets (the QA state machine) and the answer cache
internal/config   env → Config, with defaults

web/              index.html · docs.html · dev.html · deploy.html (Go templates,
                  shared head in docsbase.html) + embed.go + assets.go
web/app/          the app shell — native ES modules, no build step
                  app.js · chat.js · answer.js · viewport.js · library.js ·
                  session.js · qa.js
web/howitworks.mmd  the "how it works" diagram, authored as mermaid
web/howitworks.svg  …rendered once by `make diagram`; mermaid never ships
web/llms.go       generates /llms.txt from the rendered pages (llmstxt.org)
web/vendor.sha384 the one pin list: versions + digests, for the page and `make vendor`
web/vendor/       `make vendor` output (gitignored)
```

**Where do I add…?**

| …this | goes here | because |
|---|---|---|
| a new API endpoint | `internal/server` | routing and transport live in one place |
| a new dependency version | `web/vendor.sha384` | one line drives the URL, the digest and vendoring |
| better retrieval / prompting | `internal/rag` | the domain never imports HTTP |
| a new provider | `internal/ai` | one client, one seam; probe it with `make live` |
| a provider quirk to survive | `internal/aitest` | fault injection lives with the fake, not in each test |
| a UI action | `web/app/app.js` | it's intent; plumbing lives in its own module |
| a fetch/stream concern | `web/app/chat.js` | the app must not learn about SSE |
| anything about the corpus | `web/app/library.js` | one place decides ready / empty / unavailable |
| what survives a reload | `web/app/session.js` | storage, quota and schema drift, hidden |
| a ticket state or transition | `internal/db/tickets.go` | one table, one state machine, all four states reachable |
| anything about answer cost | `internal/db/cache.go` + `rag.Answer` | one cache, keyed on the corpus signature |
| a BA/DEV screen | `web/index.html` + `web/app/qa.js` | the loop's transport in one module, the markup in library recipes |
| markdown / citation rendering | `web/app/answer.js` | sanitising is one file's job |
| a mobile viewport quirk | `web/app/viewport.js` | keyboard/dock/scroll maths, hidden |
| a layout rule | `web/app/styles.css` | 8bit-nes owns components; this owns layout |
| a diagram | `web/*.mmd` + `make diagram` | mermaid is the source; the committed SVG is what ships |
| a guide section | one `<section>` in that role's page | both languages inline; the toggle is CSS-only |
| a whole guide page (new role) | a field on `web.Nav` + a render func + one line in `cmd/rendocs` | the guide is static, so the app needs no change at all |
| something an AI agent must know | [`AGENTS.md`](AGENTS.md) | `llms.txt` is generated; this is the hand-written part, and its pins are tested |

### The two seams that make it testable

`internal/server` depends on narrow interfaces, not on the engine — split by side, so
a test of the read path fakes nothing it doesn't use:

```go
type Answerer interface {                       // read
    Answer(ctx context.Context, a rag.Ask) (rag.Reply, error)
    Corpus(limit int) (db.Corpus, error)
}
type Knowledge interface {                      // the QA loop — optional (nil = no write routes)
    Queue(limit int) (db.Queue, error)
    OpenTicket(question, miss string) (db.Ticket, error)
    Draft(id int64, answer string) (db.Ticket, error)
    Confirm(ctx context.Context, id int64, answer string) (db.Ticket, error)
    Reject(id int64, note string) (db.Ticket, error)
    History(limit int) ([]db.Cached, error)
}
```

So the whole HTTP surface — SSE framing, cache headers, the write gate, 400s — is
covered by `go test` with a fake, needing no API key and no database. On the front
end, the same idea: `app.js` never sees an `AbortController`, a `TextDecoder`, a
`ResizeObserver` or `visualViewport`, and never learns where the BA password is kept.
It says

```js
const run = ask(question, { onToken, onCitations, onDone, fresh });  // chat.js
run.stop();                                            // resolves, never throws
await qa.act(id, "confirm", { answer });               // qa.js: throws WrongPass on 401/403
view.scrollToEnd();                                    // viewport.js: follows, unless the reader scrolled up
```

`make check` runs the lot (`gofmt`, `go vet`, `go test`, plus a guard that refuses
to let anything credential-shaped into a tracked file).

### Testing against a provider

The unit and pipeline tests need no key and no network: `internal/aitest` serves a
**fake OpenAI-compatible provider** over `httptest`, and the real `ai.Client` talks
to it. That covers the parts a new provider actually breaks — request encoding,
`index`-ordered embeddings, SSE frame parsing — which a hand-written fake client
would not. It can also misbehave on purpose: no `/embeddings`, shuffled indexes, a
short response, a mid-stream error, a 5xx.

Two commands reach for the real thing:

| command | what it answers |
|---|---|
| `make live` | Does this provider speak both endpoints? What embedding width does it return? Does chat stream incrementally? |
| `make smoke` | The whole product: ingest a fixture, ask a question only that document can answer, and check the reply streams, cites the file, and quotes a fact unique to it. |

Both read `AI_API_KEY` from `.env`, so the key never lands on a command line. Point
them at a new provider *before* ingesting anything real — `make live` will tell you
in seconds if `/embeddings` is missing, which is the one gap that stops ingest dead.

### HTTP surface

| route | returns |
|---|---|
| `GET /` | the UI (revalidated with an ETag — it pins the asset versions) |

| `GET /api/health` | `{"ok":true}` — drives the light in the top bar |
| `POST /api/chat` | SSE: `token` · `citations` · `done`, or `error` |
| `GET /api/corpus` | `{docs,chunks,approved,documents[]}` — what is indexed |
| `GET /app/…` | app modules + CSS, one ETag over the tree |
| `GET /vendor/…` | vendored assets, `immutable` (the version is in the path) |

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
  when the answer isn't there, and to cite every claim `[n]`. The UI turns those
  markers into links down to the numbered source list, so a claim is one tap from
  the file it came from.
- **You can see the corpus.** The empty state reports what is indexed and lists it
  (`GET /api/corpus`). "Nothing is ingested yet" and "retrieval found nothing" used
  to look identical — both just answered *not in the documents*.
- **The thread survives a reload.** Conversations are kept in `localStorage` and
  flushed on `pagehide`, because a phone backgrounds a tab far more often than it
  closes one.

## The QA loop: a gap becomes a document

The most useful thing the system does is turn a question it *couldn't* answer into
one it can, without anyone editing a file by hand:

```
DEV asks → answer wrong/missing → "Ask BA"      ticket: open
BA answers → "Confirm into knowledge"           docs/qa/ticket-N.md, indexed, approved
next DEV asks → retrieved with a citation       and free on the repeat
```

- **Four states, each reached by one action:** `open` · `answered` (a draft that
  survives a backgrounded phone) · `confirmed` · `rejected` (with the reason, so a
  question is never just swallowed). Asking the same question twice returns the same
  ticket, so one gap is one item on the BA's list.
- **A confirm writes a file, not a database row.** `CORPUS_DIR/qa/ticket-N.md` is
  reviewable in a diff, lives in git with the rest of the documents, and is what keeps
  `knowledge.db` derived — `ingest docs` rebuilds everything.
- **Confirmed chunks are `approved`**, which is what the long-dormant retrieval boost
  was for: the one part of the corpus a named human vouched for wins a tie.
- **Reads are open, publishing is not.** `BA_PASS` gates confirm and dismiss. Empty
  means the instance has *no* write surface at all — forgetting a secret must not be
  how you end up without one.

## Repeat questions are free

An answer is cached under the question and a **corpus signature** (document count,
chunk count, newest `updated_at`). A repeat skips *both* provider calls — no
embedding, no completion — and the UI marks it `CACHED` so a free answer never looks
like a paid one.

Any ingest, including a BA confirm, changes that signature and drops the whole cache,
so a cached answer can never outlive the sources it cites. There is no TTL to tune.
Misses and cut-off streams are never cached: those are exactly what someone retries,
and remembering them would make one bad answer permanent. Regenerate always pays.

## Caveats (read before first build)

- **sqlite-vec Go bindings evolve.** If `go mod tidy` or the build complains
  about the `github.com/asg017/sqlite-vec-go-bindings` version or the
  `sqlite_vec.Auto()` / `SerializeFloat32` API, check the current README for
  that module and adjust the version in `go.mod` / the calls in
  `internal/db/store.go`.
- **FTS5 build tag.** Uses `-tags sqlite_fts5`. If your `go-sqlite3` version
  wants `-tags fts5` instead, change it in the `Makefile`.
- **Access control is minimal.** The server binds `127.0.0.1` by default and offers
  optional HTTP Basic auth (`AUTH_PASS`); there are no accounts, no per-user
  permissions and no audit trail. The one privileged action — a BA confirming an
  answer into the corpus — is gated by a second shared password (`BA_PASS`), which is
  a permission boundary, not an identity. To let the team reach it from anywhere,
  publish it through a tailnet or a Cloudflare Tunnel rather than opening a port — see
  the **[Deploy page](https://mega-hub-studio.github.io/mega-docs/deploy.html)**.

## Frontend assets (CDN, pinned)

The UI is one embedded HTML file that pulls four things from jsDelivr: Vue,
`marked`, DOMPurify, and the [8-BIT NES](https://github.com/TuTranMVP/8bit-components)
design system (**0.7.0**). All of it is version-pinned *and* hash-pinned:

| | pin |
|---|---|
| one origin | a single `preconnect`ed host → one DNS + TLS handshake for every asset |
| exact versions | `vue@3.5.40`, `marked@18.0.7`, `dompurify@3.4.12`, `8bit-nes@0.7.0` — a pinned jsDelivr URL is `immutable`, cached a year, and can't change under a deployed page |
| `integrity` | `sha384` on all four; the browser refuses a byte that doesn't match |
| `defer` + module | ~240 kB of `<script>` no longer blocks the parser; the app boots from the inline module, which runs after them by spec |
| font `preload` | the three woff2 faces start with the stylesheet, at the exact URLs the CSS resolves — each fetched once |

**One line to upgrade.** [`web/vendor.sha384`](web/vendor.sha384) is the only place
a version or a digest appears: `index.html` asks for
`url "vue" "dist/vue.global.prod.js"` and Go resolves it from that manifest, which
also drives `make vendor`. Change the line, run `make vendor`, done — and a
half-finished bump (one file moved, the rest left behind) fails at **startup**, not
in someone's browser. 8-BIT NES publishes its own digests at
[`/sri.json`](https://tutranmvp.github.io/8bit-components/sri.json).

> **On 8bit-nes 0.7.0** (released 2026-07-26): the bump is one line plus
> `make vendor`. Everything here is verified against the published tarball rather
> than the changelog, because a release has already once been tagged without the fix
> it claimed — all five digests match the package's own `sri.json`.
>
> 0.7.0 **keeps** 0.6.1's two touch fixes (send button 44px, chat textarea 16px), so
> `web/app/styles.css` still owns no `(pointer: coarse)` CSS.
>
> It does **not** carry two accessibility fixes this project found and reported
> upstream, so `web/docsbase.html` still overrides them locally, each marked with why:
> `.wt-dot` is a 12×12 tap target, and `base.css` has no `a` rule at all so an inline
> link is visually identical to the text around it. Both confirmed by pinning 0.7.0
> and removing the overrides: the dots drop to 12×12 and links lose their underline
> and go back to body colour. They stay until a release carries them.

> **On the diagram.** The "how it works" picture is a mermaid graph, but mermaid
> never reaches the browser: 8bit-nes deliberately does not bundle it (~800KB
> gzipped, and its ESM build chunk-splits, so it cannot be SRI-pinned or vendored
> for an air-gapped box). `make diagram` renders `web/*.mmd` to SVG once — themed by
> the design system's own `mermaidTheme()`, so it matches what runtime mermaid would
> have drawn — and the SVG is committed and inlined. `<nes-walkthrough>` then
> spotlights one stage at a time. Editing a `.mmd` without re-rendering is the one
> hazard of committing generated output, so the generator stamps the source hash into
> the SVG and `make check` compares it.

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
