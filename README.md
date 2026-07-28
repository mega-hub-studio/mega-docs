# mega-docs

*The binary, the database and the systemd unit are still named `knowledge` — renaming
those would break a running deployment for no user-visible gain. `mega-docs` is the
product name: what the app's header, its tab title and this documentation say.*

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

| | Guide | BA mode | Dev | Deploy |
|---|---|---|---|---|
| Answers | "what is this, can I trust this answer?" | "how do I get good answers, and answer a gap?" | "where do I change X?" | "how do we run it for the team?" |
| For | everyone, first 60 seconds | BA / PM / support, day to day | DEV | whoever hosts it |
| Published at | [/](https://mega-hub-studio.github.io/mega-docs/) | [/ba.html](https://mega-hub-studio.github.io/mega-docs/ba.html) | [/dev.html](https://mega-hub-studio.github.io/mega-docs/dev.html) | [/deploy.html](https://mega-hub-studio.github.io/mega-docs/deploy.html) |

The Guide opens with a router and a 60-second quick start, so nobody has to read a page
that is not theirs. Each page is split into one section per feature, indexed by
`<nes-toc>`; the two flows a reader has to hold in their head — how an answer is built,
and how a gap becomes a document — are diagrams, rendered at build time from `web/*.mmd`.

**The pages are also the spec.** A section that maps to code declares the mapping in its
own markup — `data-feature`, `data-api`, `data-env`, `data-test` — and
[`/spec.json`](https://mega-hub-studio.github.io/mega-docs/spec.json) is generated from
those annotations: every feature with the routes, environment variables and tests behind
it, and a link to the prose that defines it. The join is checked both ways, so an `/api/`
route or a config variable that no section documents **fails `make check`**. That is the
whole of "docs are the source of truth": write the section, declare the join, watch it go
red, then implement. The steps are on the Dev page under *These pages are the spec*.

For AI agents there is [`AGENTS.md`](AGENTS.md), the `spec.json` above, and a generated
[`/llms.txt`](https://mega-hub-studio.github.io/mega-docs/llms.txt) — an
[llmstxt.org](https://llmstxt.org) index of every page and section, built by
`cmd/rendocs` from the pages themselves so it cannot drift from them. `AGENTS.md`
also points at the **version-pinned** 8bit-nes docs (`llms.txt` ships inside the
package, so a jsDelivr URL is version-exact, unlike the always-latest docs site) and
a test fails if that version drifts from `web/vendor.sha384`.

**Guide** is the first read: 60 seconds to a first answer, how it works as a diagram,
what happens when the answer isn't there, what the app can do, getting in the first
time, and the failures that actually happen. **BA mode** is the day-to-day, seven
numbered moves in the order they come up: ask so retrieval finds it, decide whether to
trust the answer, narrow to one folder, file and answer a gap, import documents, read
what an answer cost, and what to do when something looks wrong. **Dev** is the first
hour: where things live, the two seams, how an answer is built, the HTTP surface,
testing with no API key, the knobs people actually turn, and the four layers of the
front end — it points here for the full reference rather than restating it. **Deploy**
is the first install, letting the team in (Tailscale / Cloudflare Tunnel / LAN), where
to run it, systemd, backups and the settings table.

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

internal/server   HTTP: routes, cache policy, SSE, the BA write gate. No SQLite.
internal/rag      the domain: chunk → embed → retrieve → grounded answer
internal/ai       one OpenAI-compatible client (embeddings + chat streaming)
internal/aitest   a fake provider over httptest — the whole pipeline, no key needed
internal/db       SQLite: sqlite-vec + FTS5, hybrid search with RRF,
                  tickets (the QA state machine) and the answer cache
internal/config   env → Config, with defaults

web/              docs.html · ba.html · dev.html · deploy.html — the guide
                  (Go templates, shared head in docsbase.html) + embed.go + assets.go
                  *.mmd + *.svg  diagram sources and their committed renders
                  spec.go        the pages' annotations → the published spec.json
web/dist/         the built app: committed, embedded, served. `make ui` regenerates it
web/ui/           the app's front end — Vue 3.5 SFCs, JavaScript, built by Vite
                  index.html · vite.config.js · package.json · eslint.config.js
                  src/main.js       createApp + mount, and the one design-system import
                  src/App.vue       wiring: composables in, components placed
                  src/components/   ChatTurn · EmptyScreen · ScopePicker · StatusLine
                                    BaScreen · ImportPanel · TicketCard · CorpusTree
                  src/lib/          plumbing — no Vue import anywhere in here
                                    chat.js · qa.js · upload.js   transport (SSE, tickets, import)
                                    answer.js · diagram.js        rendering (markdown, mermaid)
                                    library.js · session.js · viewport.js  corpus, storage, keyboard
                  src/styles.css    layout only; 8bit-nes owns the components
web/ui/src/composables/   one composable per concern, and nothing else in them
                  conversation.js  turns, ask/regenerate/stop/reset, persistence
                  corpus.js        what is indexed + the starters derived from it
                  scope.js         which folder answers
                  qaloop.js        the ticket queue and the free-to-replay history
                  runtime.js       online · writes · model, prices and the guide's address
                  statusline.js    the bottom strip, as one computed object
                  diagrams.js      the lazy renderer and the zoom viewer
                  gate.js          the BA password, and what a refused write means
                  importer.js      files in, progress, per-file results
                  tickets.js       four states, one path, one draft per ticket
                  nestree.js       the <nes-tree> payload and its rebuild rules

**Where do I add…?**

| …this | goes here | because |
|---|---|---|
| a new API endpoint | `internal/server` | routing and transport live in one place |
| a new dependency version | `web/vendor.sha384` | one line drives the URL, the digest and vendoring |
| better retrieval / prompting | `internal/rag` | the domain never imports HTTP |
| how a document is cut into sections | `internal/rag/chunk.go` | small sections merge, oversized ones split by paragraph then by line; **changing it needs a re-ingest** |
| a new provider | `internal/ai` | one client, one seam; probe it with `make live` |
| a provider quirk to survive | `internal/aitest` | fault injection lives with the fake, not in each test |
| a UI action | `web/ui/src/App.vue` | it's wiring and intent; state lives in a composable, plumbing in its own module |
| a new piece of shell state | a file in `web/ui/src/composables/` | one concern per composable — the shell used to be one object where the import bar could collide with the chat thread |
| a fetch/stream concern | `web/ui/src/lib/chat.js` | the app must not learn about SSE |
| anything about the corpus | `web/ui/src/lib/library.js` | one place decides ready / empty / unavailable |
| what survives a reload | `web/ui/src/lib/session.js` | storage, quota and schema drift, hidden |
| a ticket state or transition | `internal/db/tickets.go` | one table, one state machine, all four states reachable |
| anything about answer cost | `internal/db/cache.go` + `rag.Answer` | one cache, keyed on question + scope, under a signature of corpus + chat model + prompt |
| retrieval scope (ask inside a folder) | `db.Search`'s scope filter + `rag.Scope` + `components/CorpusTree.vue` | filtered before both retrievers rank, canonicalised in one place because it is half the cache key |
| a BA-screen behaviour | `web/ui/src/composables/gate.js`, `importer.js` or `tickets.js` | the screen is headless — 54 lines of props, emits and composition; the behaviour is in one of these three |
| a chat-screen behaviour | `web/ui/src/composables/conversation.js` | the thread owns ask/stream/stop/reset; `App.vue` only wires it |
| document import | `internal/rag/upload.go` + `internal/server/documents.go` + `web/ui/src/lib/upload.js` | path validation next to the writer, transport next to the form |
| markdown / citation rendering | `web/ui/src/lib/answer.js` | sanitising is one file's job |
| a mobile viewport quirk | `web/ui/src/lib/viewport.js` | keyboard/dock/scroll maths, hidden |
| a layout rule | `web/ui/src/styles.css` | 8bit-nes owns components; this owns layout |
| a layout rule on the **docs** pages | `web/docsbase.html` | one `--flow` gap for every block; run `make check-ui` after |
| a diagram | `web/*.mmd` + `make diagram` | mermaid is the source; the committed SVG is what ships |
| a feature | its page `<section>` **first**, with `data-feature`/`data-api`/`data-env`/`data-test` | `make check` is red until those names exist — the docs are the input, not the write-up |
| a guide section | one `<section>` in that role's page | both languages inline; the toggle is CSS-only |
| a sub-module inside a section | an `<h3>` in that section | `<nes-toc>` indexes h2+h3, so it appears in the rail on its own |
| a whole guide page (new role) | a field on `web.Nav` + a render func + one entry in `web.Pages` | `cmd/rendocs`, `llms.txt` and `spec.json` all walk that one list, so they cannot publish different sets |
| something an AI agent must know | [`AGENTS.md`](AGENTS.md) | `llms.txt` is generated; this is the hand-written part, and its pins are tested |

### The two seams that make it testable

`internal/server` depends on narrow interfaces, not on the engine — split by side, so
a test of the read path fakes nothing it doesn't use:

```go
type Answerer interface {                       // read
    Answer(ctx context.Context, a rag.Ask) (rag.Reply, error)
    Corpus(limit int) (db.Corpus, error)
}
type Importer interface {                       // documents in — optional, same gate
    Import(ctx context.Context, dir string, files []File) (Result, error)
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
| `GET /api/health` | `{ok,writes,model,window,price_in,price_out}` — the light in the top bar, whether BA mode can publish, and what the status line reports |
| `POST /api/chat` | `{question, scope?, fresh?}` → SSE: `token` · `citations` · `done{cached,in,out}`, or `error` |
| `GET /api/corpus` | `{docs,chunks,approved,documents[]}` — what is indexed, with full paths |
| `GET · POST /api/tickets` | read the queue · file a gap |
| `POST /api/tickets/{id}/{action}` | `draft` · `confirm` · `reject` — needs `X-BA-Pass` |
| `POST /api/documents` | multipart import: `files` (repeatable) + optional `dir` — needs `X-BA-Pass` |
| `GET /api/history` | answers still free to replay, with hit counts and the scope each was answered in |
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
- **Reads are open, publishing is not.** `BA_PASS` gates confirm, dismiss and
  `POST /api/documents`. Empty means the instance has *no* write surface at all —
  forgetting a secret must not be how you end up without one.
- **Two write paths, one identity rule.** A BA confirm writes `qa/ticket-N.md`; a
  browser import writes the path the file was given, folders kept, validated per
  segment (`rag.SafePath`: no `..`, no hidden or absolute segment, max 4 deep, and
  `qa/` refused so an import cannot impersonate a vouched answer). Both then take the
  same path identity as `cmd/ingest`, so CLI and browser cannot disagree.

## Repeat questions are free

An answer is cached under the question and a **corpus signature** (document count,
chunk count, newest `updated_at`). A repeat skips *both* provider calls — no
embedding, no completion — and the UI marks it `CACHED` so a free answer never looks
like a paid one.

Any ingest, including a BA confirm, changes that signature and drops the whole cache,
so a cached answer can never outlive the sources it cites. There is no TTL to tune.
Misses and cut-off streams are never cached: those are exactly what someone retries,
and remembering them would make one bad answer permanent. Regenerate always pays.

## Upgrading a running instance

```bash
cd /opt/knowledge && git pull origin main
make build                              # the UI is compiled in — pulling alone changes nothing
sudo systemctl restart knowledge
curl -s localhost:8080/api/health       # {"ok":true,"writes":true} — writes:true = BA_PASS is set
```

No Node on the host and none needed: the front end is built by `make ui` into
`web/dist`, which is committed, and `make build` embeds whatever is there.

**One upgrade needs a re-index.** Document paths are now stored relative to
`CORPUS_DIR`. An index built before that stored them differently, so a re-ingest would
add a *second* document for every file — each cited twice. Check what yours holds:

```bash
sqlite3 knowledge.db 'select path from documents limit 3'
# spec.md · qa/ticket-1.md              → already relative, nothing to do
# docs/spec.md · /opt/knowledge/docs/…  → predates the change, rebuild:
rm -f knowledge.db knowledge.db-wal knowledge.db-shm && ./bin/ingest docs
```

Deleting the index is safe: it is derived. `CORPUS_DIR` is the source of truth,
confirmed answers included, and the answer cache rebuilds itself on demand. The only
cost is re-embedding the corpus.

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

## Frontend assets: two manifests, one design system

Both front ends use [8-BIT NES](https://github.com/TuTranMVP/8bit-components) (**0.7.3**),
and each gets it a different way — which is the whole reason there are two manifests and
neither knows the other's versions:

| | the app (`web/ui`) | the guide (`web/*.html`) |
|---|---|---|
| how it arrives | npm, bundled by Vite into `web/dist` — committed, embedded, served from the binary | jsDelivr at runtime: there is no build step, just four static files on GitHub Pages |
| pinned in | [`web/ui/package.json`](web/ui/package.json), bytes in `package-lock.json` | [`web/vendor.sha384`](web/vendor.sha384) |
| verified by | `npm ci` against the lockfile; CI rebuilds and diffs `web/dist` | `integrity="sha384-…"`, so the browser refuses a byte that doesn't match |

What the guide's pins buy, since it is the half that fetches anything at all: one
`preconnect`ed origin for every asset; an exact version, because a pinned jsDelivr URL is
`immutable` and cannot change under a deployed page; and a `preload` for all three woff2
faces, at the exact URLs the CSS resolves, so each is fetched once and starts during head
parse rather than after the stylesheet.

**One line to upgrade the guide.** `web/vendor.sha384` is the only place its versions and
digests appear — `docsbase.html` asks for `url "8bit-nes" "all.min.css"` and Go resolves
both the URL and its `sri` from that manifest. Change the version, run `make vendor` for
the digests, done; a half-finished bump fails at **startup**, not in someone's browser.
8-BIT NES publishes its own digests at
[`/sri.json`](https://tutranmvp.github.io/8bit-components/sri.json), and `AGENTS.md` quotes
the version too — a test fails when the two disagree.

> **On 8bit-nes 0.7.3** (released 2026-07-28): fixes a bug this project reported after
> measuring it on a phone. Setting an observed attribute rebuilds `<nes-toc>`'s index, and
> the component kept its active heading id across the rebuild — so `_mark()` skipped the
> fresh "current section" span and left it blank. These pages write both `label` and
> `levels` on a language toggle, which is two rebuilds back to back, and the collapsed bar
> (which on a phone *is* the index) went empty on every page after one tap on VI/EN.
>
> Upstream now forgets the id when it rebuilds, so `docsbase.html`'s workaround — writing
> the two attributes in a load-bearing order — is gone. Only `elements.min.js` moved;
> `all.min.css` and the three fonts are byte-identical to 0.7.2. Digests verified against
> the published tarball rather than the changelog, because a release has already once been
> tagged without the fix it claimed.
>
> Earlier releases left their fixes behind, and they are why this repo overrides less than
> it used to: 0.7.1 carried both accessibility fixes reported from here (prose links get
> ink colour plus an underline; a `.wt-dot` grows a 40px hit area on a coarse pointer), so
> `web/docsbase.html` overrides nothing, and 0.6.1's two touch fixes (send button 44px,
> chat textarea 16px) are why `web/ui/src/styles.css` owns no `(pointer: coarse)` CSS.

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

Nothing to do: the app fetches nothing at runtime. Vue, the design system, marked,
DOMPurify and mermaid are npm dependencies of `web/ui`, bundled by Vite into `web/dist`
and served from the binary — content-hashed, so `/assets/` goes out `Cache-Control:
immutable`. Even the diagram renderer is a lazy *chunk* rather than a CDN fetch: it loads
the first time an answer actually contains a diagram, from the same origin.

`make vendor` still exists, and it is the **guide's**: four static pages on GitHub Pages
with no build step, loading the design system from jsDelivr with an `integrity` attribute.
Vendoring lets you render and read them with no egress at all:

```bash
make vendor                                       # fetch + sha384-verify into web/vendor/
go run ./cmd/rendocs -d /tmp/site -base /vendor   # the guide, pointing at local copies
```
