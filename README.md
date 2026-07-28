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

Everything ships in the `knowledge` binary (the Vue app is embedded via `go:embed`).
No Node, no Docker, no external vector DB.

**This file is the reference.** The rules are in [`CLAUDE.md`](CLAUDE.md), the settled
decisions in [`changelog/`](changelog/), and what an agent gets wrong in
[`AGENTS.md`](AGENTS.md) — none of it is repeated here.

## Prerequisites

- Go 1.22.5+
- A C compiler (gcc/clang) + `sqlite3.h` (Debian/Ubuntu: `apt install libsqlite3-dev`;
  macOS: preinstalled with Xcode CLT) — required by the cgo SQLite bindings.
- An OpenAI API key. Any OpenAI-compatible base URL works (Azure, Groq, OpenRouter),
  which is a property of the wire format rather than a feature maintained here.

## Quick start

```bash
cp .env.example .env      # then set AI_API_KEY; everything else is already its default
make deps                 # go mod tidy

# 1) Put at least one document in docs/ — the folder ships EMPTY (a .gitkeep and
#    nothing else), and `ingest` exits non-zero on "nothing was indexed" rather than
#    reporting success over an empty index.
cp README.md docs/        # or your own .md / .txt files

# 2) Index it (folder or files; .md/.txt only)
make ingest DOCS=./docs

# 3) Start the chat server
make server               # http://localhost:8080
```

Ship it as a single binary instead — the binary *is* the web server, so there is no
frontend to deploy separately:

```bash
make build && ./bin/knowledge
```

## Now vs vNext

[`README-MEGA-DOCS.md`](README-MEGA-DOCS.md) is the **brief for the product this is
becoming** — a Knowledge Engine Platform with three roles and the WebUI as the only way
in. It is not a description of this tree, and this table is the only place the two are
joined. Read it before implementing anything from the brief: most of what is not shipped is
**decided rather than queued**, and several lines are settled decisions *against* the brief.
Re-deriving any of them lands on the wrong answer.

A ✅ **decided** row is closed: either it shipped, or it will not be built until its stated
trigger exists. Only 🟡 rows are work, and each says what the remaining part is. Nothing here
is a backlog item to find and start.

| the brief asks for | today | |
|---|---|---|
| OpenAI only; no local LLMs or embeddings | one OpenAI-compatible client, no other SDK | ✅ **shipped** |
| Never render raw HTML | `marked → DOMPurify`, then components built from *parsed values* | ✅ **shipped** |
| Markdown Components → NES Renderer | `dressTables · dressTaskLists · linkCites · asDiagrams` | ✅ **shipped** |
| Reuse NES before creating components | app CSS owns exactly two overrides, both named in `AGENTS.md` | ✅ **shipped** |
| PDF / DOCX upload | **out of scope** — `.md · .markdown · .txt`, and the refusal names the converter | ✅ **decided** |
| One pipeline, no hybrid retrieval | **BM25 stays** — vector KNN + BM25, fused with RRF | ✅ **decided** |
| WebUI is the single entry point | `Upload` is the only *BA* path in; `ingest` is an operator recovery tool | 🟡 **partial** |
| Three roles (Admin · BA · DEV) | the two that exist are expressed once each — DEV reads (open), BA writes (`BA_PASS`). **`ADMIN_PASS` deliberately not added**: there is no admin-only *action* to gate, so it would be a knob with no job (rule 20). Trigger: the first admin-only action | ✅ **decided** |
| BA verbs: Upload · CRUD · Preview · Version · Publish · Archive · Reindex | create, update and delete ship — import, re-import the same path to replace, and remove behind the library's `.perm` confirmation in `ImportPanel.vue` (the file goes to `docs/.trash/`). Preview · Version · Publish · Archive · Reindex: not built | 🟡 **partial** |
| Response Format: Answer · Visual Components · References · Related Documents · Suggested Actions | the first three ship — `rag.Reply{Citations,…}`, and `lib/answer.js` renders tables, task lists and diagrams from NES recipes. The last two are **not being built on a name**: "Suggested Actions" could be model follow-ups, presets or ticket shortcuts. Trigger: one sentence saying what they contain | ✅ **decided** |
| Knowledge Model: Document · Sections · Chunks · Embeddings · References · Tags · Categories · Relations · Version | `documents`, `chunks`, embeddings and citations exist. The other five are new *tables*, so they cost no re-ingest whenever they land — which is why there is no hurry and no schema for them yet. Trigger: a BA screen that filters by one | ✅ **decided** |
| Removed: Git sync · Folder watch | both gone — `scripts/corpus-sync.sh` deleted with its `.path` and `.timer` units. **There is no backup story at all**, by decision, not by omission | ✅ **decided** |
| Cross-OS self-host (WSL2 · macOS) | one binary, no runtime; the tooling was already portable (`openssl dgst`, not `sha384sum`). Both supervisors documented on the Deploy page | ✅ **shipped** |
| Knowledge DB is the source of truth | `CORPUS_DIR` is; the DB is derived (invariant 1). **Nothing blocks the switch any more** — the migration runner shipped and the backup precondition was dropped | 🟡 **next** |

The rows worth reading the reasoning for:

- **Inverting the source of truth** is unblocked, and both preconditions are settled rather
  than pending. The **migration runner** shipped (`internal/db/migrate.go` — forward only,
  one transaction per migration, `schema_version` as a table), landing *before* the corpus
  directory stops being written to because the other order removes the way back. The
  **backup precondition was dropped**: the WebUI import is the one controlled way a document
  enters, so it is the control point the brief asked for, and a second copy of the corpus is
  not what makes that true. What is left is the work itself — stop writing files, let
  `Upload` write to the DB — and nothing gates it.
  → [`changelog/2026-07-28-no-backup.md`](changelog/2026-07-28-no-backup.md)
  · [`sot-decision`](changelog/2026-07-28-sot-decision.md)
- **PDF/DOCX is out of scope**, not pending. A Go parser puts a binary-format parser's CVE
  surface inside a service with a write gate and no accounts; an external converter run at
  upload is a per-file failure a BA cannot fix. Converting stays a one-time step *outside*
  the product, and the DX that makes that acceptable already exists: `upload.go` names the
  command in its refusal instead of reporting an unsupported type.
- **Dropping BM25 is rejected.** It is the half that matches an error code, a config key or
  a rule id verbatim over a Vietnamese corpus (`unicode61 remove_diacritics 2`), and the BA
  guide's own advice is that an error code beats a paraphrase. "One pipeline" is already
  true in the sense that matters — one path a question travels, one `Answer`. Invariant 4
  and `TestScopedSearchRanksWithinTheScope` stay with it.
- **Git sync and folder watch are gone**, and with them the only thing that backed the
  corpus up. That was raised as a risk and accepted deliberately: the brief removes both,
  and carrying automation the brief deletes in order to protect a backup the brief does not
  ask for is the redundancy this cleanup exists to remove. **Nothing replaces it**: the
  backup story is gone rather than pending, because every document enters through one
  controlled path (the BA WebUI import) and `Remove` is a soft delete into `docs/.trash/`.
  → [`changelog/2026-07-28-drop-corpus-sync.md`](changelog/2026-07-28-drop-corpus-sync.md)
  · [`scope-decisions`](changelog/2026-07-28-scope-decisions.md)
  · [`vnext-collisions`](changelog/2026-07-28-vnext-collisions.md)

## The published guide

Bilingual (EN/VI) static pages on their own domain, one per role, because each role
arrives with a different question:

| | Guide | BA mode | Dev | Deploy |
|---|---|---|---|---|
| Answers | "what is this, can I trust this answer?" | "how do I get good answers, and answer a gap?" | "where do I change X?" | "how do we run it for the team?" |
| For | everyone, first 60 seconds | BA / PM / support, day to day | DEV | whoever hosts it |
| Published at | [/](https://mega-hub-studio.github.io/mega-docs/) | [/ba.html](https://mega-hub-studio.github.io/mega-docs/ba.html) | [/dev.html](https://mega-hub-studio.github.io/mega-docs/dev.html) | [/deploy.html](https://mega-hub-studio.github.io/mega-docs/deploy.html) |

**The pages are also the spec.** A section that maps to code declares the mapping in its
own markup — `data-feature`, `data-api`, `data-env`, `data-test` — and
[`/spec.json`](https://mega-hub-studio.github.io/mega-docs/spec.json) is generated from
those annotations. The join is checked both ways, so an `/api/` route or a config variable
that no section documents **fails `make check`**: write the section, declare the join,
watch it go red, then implement.

For agents there is [`AGENTS.md`](AGENTS.md), that `spec.json`, and a generated
[`/llms.txt`](https://mega-hub-studio.github.io/mega-docs/llms.txt) —
an [llmstxt.org](https://llmstxt.org) index built by `cmd/rendocs` from the pages
themselves, so it cannot drift from them.

The app binary does **not** serve the guide, and must not learn how: a second copy inside
the app is noise on the surface people came to use, plus a copy to drift. It does not link
out to it either — the guide is published from here on its own cadence, and an address
compiled into a running server is one more thing to update when it moves. Build the pages
locally with `go run ./cmd/rendocs -d /tmp/site -base /vendor`.

*Published by CI on every push to `main` that touches the guide. If you fork this: turn
Pages on once under Settings → Pages → Source **GitHub Actions**. A workflow token cannot
do it for you — creating a Pages site needs repo-admin rights — and until it is on, every
run fails at `configure-pages`.*

## Architecture

One binary, one direction of dependency — `cmd` → `internal` → nothing. The layer rules
and the seams are in [`CLAUDE.md`](CLAUDE.md#architecture); this is the map.

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
                  migrate.go  forward-only versioned migrations; read its header first
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
                  src/router.js     the two screens, as addresses: /#/ask and /#/ba
                  src/components/   AskScreen · BaScreen   the two screens the router picks
                                    ChatTurn · EmptyScreen · ScopePicker · StatusLine
                                    ImportPanel · TicketCard · CorpusTree
                  src/lib/          plumbing — no Vue import anywhere in here
                                    chat.js · qa.js · upload.js   transport (SSE, tickets, import)
                                    answer.js · diagram.js        rendering (markdown, mermaid)
                                    library.js · session.js · viewport.js  corpus, storage, keyboard
                                    i18n.js        the EN/VI catalogues + the stored choice
                  src/styles.css    layout only; 8bit-nes owns the components
                  src/composables/  one concern each, and nothing else in them
                                    conversation.js  turns, ask/regenerate/stop/reset, persistence
                                    corpus.js        what is indexed
                                    scope.js         which folder answers
                                    qaloop.js        the ticket queue and the free-to-replay history
                                    runtime.js       online · writes · model, prices and the guide's address
                                    statusline.js    the bottom strip, as one computed object
                                    diagrams.js      the lazy renderer and the zoom viewer
                                    gate.js          the BA password, and what a refused write means
                                    importer.js      files in, progress, per-file results
                                    tickets.js       four states, one path, one draft per ticket
                                    nestree.js       the <nes-tree> payload and its rebuild rules
                                    finder.js        the first screen's document menu: one query, ranked rows
                                    lang.js          useT(): the only door to i18n a component may use
```

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
| a screen, or where one lives | `web/ui/src/router.js` | the address is the only truth for which screen is showing; the shell still owns the state both read |
| a new piece of shell state | a file in `web/ui/src/composables/` | one concern per composable — the shell used to be one object where the import bar could collide with the chat thread |
| a fetch/stream concern | `web/ui/src/lib/chat.js` | the app must not learn about SSE |
| anything about the corpus | `web/ui/src/lib/library.js` | one place decides ready / empty / unavailable |
| what survives a reload | `web/ui/src/lib/session.js` | storage, quota and schema drift, hidden |
| a ticket state or transition | `internal/db/tickets.go` | one table, one state machine, all four states reachable |
| a new **table** | `internal/db/schema.sql` | `CREATE TABLE IF NOT EXISTS` does reach an existing database |
| a new **column** | a migration in `internal/db/migrate.go` | `IF NOT EXISTS` finds the table and does nothing, so the column never arrives — never renumber an id, never edit shipped SQL |
| anything about answer cost | `internal/db/cache.go` + `rag.Answer` | one cache, keyed on question + scope, under a signature of corpus + chat model + prompt |
| retrieval scope (ask inside a folder) | `db.Search`'s scope filter + `rag.Scope` + `components/CorpusTree.vue` | filtered before both retrievers rank, canonicalised in one place because it is half the cache key |
| a BA-screen behaviour | `web/ui/src/composables/gate.js`, `importer.js` or `tickets.js` | the screen is headless — props, emits and composition; the behaviour is in one of these three |
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
| a settled decision | a file in `changelog/` | it is the only place state outside git is recorded, and the only cure for re-deriving it |
| something an AI agent must know | [`AGENTS.md`](AGENTS.md) | `llms.txt` is generated; this is the hand-written part, and its pins are tested |

### HTTP surface

| route | returns |
|---|---|
| `GET /` | the app (revalidated with an ETag — it pins the asset versions) |
| `GET /assets/…` | the bundle, `immutable`: every name carries a content hash |
| `GET /api/health` | `{ok,writes,model,window,price_in,price_out}` — the light in the top bar, whether BA mode can publish, and what the status line reports. No `site`: the binary does not link out to the guide (see `CLAUDE.md`) |
| `POST /api/chat` | `{question, scope?, fresh?}` → SSE: `token` · `citations` · `done{cached,in,out}`, or `error` |
| `GET /api/corpus` | `{docs,chunks,approved,documents[]}` — what is indexed, with full paths |
| `GET · POST /api/tickets` | read the queue · file a gap |
| `POST /api/tickets/{id}/{action}` | `draft` · `confirm` · `reject` — needs `X-BA-Pass` |
| `POST /api/documents` | multipart import: `files` (repeatable) + optional `dir` — needs `X-BA-Pass` |
| `DELETE /api/documents/{path…}` | remove a document and its chunks — needs `X-BA-Pass` |
| `GET /api/history` | answers still free to replay, with hit counts and the scope each was answered in |

`internal/server` reaches the engine through three narrow interfaces — `Answerer` (read,
required), `Knowledge` and `Importer` (writes, nil-able: their routes **disappear rather
than half-work**). Read them in [`internal/server/server.go`](internal/server/server.go)
rather than from a copy here. What they buy: the whole HTTP surface — SSE framing, cache
headers, the write gate, 400s — is covered by `go test` with fakes, needing no API key and
no database.

### Testing against a provider

The unit and pipeline tests need no key and no network: `internal/aitest` serves a fake
OpenAI-compatible provider over `httptest` and the real `ai.Client` talks to it, so the
parts a new provider actually breaks are covered — request encoding, `index`-ordered
embeddings, SSE frame parsing. It can misbehave on purpose too: no `/embeddings`, shuffled
indexes, a short response, a mid-stream error, a 5xx.

`make live` and `make smoke` reach the real thing and both read `AI_API_KEY` from `.env`,
so the key never lands on a command line. Point them at a new provider *before* ingesting
anything real: `make live` says in seconds whether `/embeddings` is missing, which is the
one gap that stops ingest dead.

## PDF / DOCX: convert first, on purpose

`.md` · `.markdown` · `.txt` is the whole list, and that is a **decision**, not a gap —
see *Now vs vNext*. Convert outside the product, then ingest the markdown:

```bash
pip install markitdown
markitdown spec.pdf > docs/spec.md
make ingest DOCS=./docs
```

`MarkItDown` (Microsoft) or `Docling` (IBM) both work well. What makes this acceptable DX
rather than an obstacle is that nothing has to be looked up: the upload refusal names the
command. A rejection that tells you the fix is not a missing feature — and it keeps "the
documents are messy" a one-time cleaning step instead of a runtime failure mode inside a
service that has a write gate.

## How answers stay grounded

- Retrieval fuses vector similarity (`sqlite-vec`) and BM25 keyword match (`FTS5`) with
  Reciprocal Rank Fusion. Keyword match is what catches function names, error codes and
  config keys that pure semantic search misses.
- The LLM answers **only** from retrieved context, says so when the answer isn't there,
  and cites every claim `[n]`. The UI turns those markers into links down to the numbered
  source list, so a claim is one tap from the file it came from.
- **You can see the corpus.** The empty state reports what is indexed and lists it
  (`GET /api/corpus`). "Nothing is ingested yet" and "retrieval found nothing" used to
  look identical — both just answered *not in the documents*.
- **The thread survives a reload.** Conversations are kept in `localStorage` and flushed
  on `pagehide`, because a phone backgrounds a tab far more often than it closes one.

## The QA loop: a gap becomes a document

The most useful thing the system does is turn a question it *couldn't* answer into one it
can, without anyone editing a file by hand:

```
DEV asks → answer wrong/missing → "Ask BA"      ticket: open
BA answers → "Confirm into knowledge"           docs/qa/ticket-N.md, indexed, approved
next DEV asks → retrieved with a citation       and free on the repeat
```

- **Four states, each reached by one action:** `open` · `answered` (a draft that survives
  a backgrounded phone) · `confirmed` · `rejected` (with the reason, so a question is
  never just swallowed). Asking the same question twice returns the same ticket, so one
  gap is one item on the BA's list.
- **A confirm writes a file, not a database row.** `CORPUS_DIR/qa/ticket-N.md` is
  reviewable in a diff and is what keeps `knowledge.db` derived.
- **Confirmed chunks are `approved`**, which is what the retrieval boost is for: the one
  part of the corpus a named human vouched for wins a tie.
- **Two write paths, one identity rule.** A BA confirm writes `qa/ticket-N.md`; an import
  writes the path the file was given, folders kept, validated per segment (`rag.SafePath`:
  no `..`, no hidden or absolute segment, max 4 deep, and `qa/` refused so an import
  cannot impersonate a vouched answer). Both then take the same path identity as
  `cmd/ingest`, so CLI and browser cannot disagree.

## Repeat questions are free

An answer is cached under the question and a **corpus signature** (document count, chunk
count, newest `updated_at`). A repeat skips *both* provider calls — no embedding, no
completion — and the UI marks it `CACHED` so a free answer never looks like a paid one.

Any ingest, including a BA confirm, changes that signature and drops the whole cache, so a
cached answer can never outlive the sources it cites. There is no TTL to tune. Misses and
cut-off streams are never cached: those are exactly what someone retries, and remembering
them would make one bad answer permanent. Regenerate always pays.

## Upgrading a running instance

```bash
cd /opt/knowledge && git pull origin main
make build                              # the app is compiled in — pulling alone changes nothing
sudo systemctl restart knowledge
curl -s localhost:8080/api/health       # {"ok":true,"writes":true} — writes:true = BA_PASS is set
```

No Node on the host and none needed: the front end is built by `make ui` into `web/dist`,
which is committed, and `make build` embeds whatever is there. Install with `mv`, not `cp`
— `cp` over a running binary fails with `Text file busy`.

**One upgrade needs a re-index.** Document paths are now stored relative to `CORPUS_DIR`.
An index built before that stored them differently, so a re-ingest would add a *second*
document for every file — each cited twice. Check what yours holds:

```bash
sqlite3 knowledge.db 'select path from documents limit 3'
# spec.md · qa/ticket-1.md              → already relative, nothing to do
# docs/spec.md · /opt/knowledge/docs/…  → predates the change, rebuild:
rm -f knowledge.db knowledge.db-wal knowledge.db-shm && ./bin/ingest docs
```

Deleting the index is safe *while invariant 1 holds*: it is derived, `CORPUS_DIR` is the
truth (confirmed answers included), and the answer cache rebuilds itself on demand. The
only cost is re-embedding the corpus. That safety is exactly what the vNext inversion
spends — see **Now vs vNext**.

## Caveats (read before first build)

- **sqlite-vec Go bindings evolve.** If `go mod tidy` or the build complains about the
  `github.com/asg017/sqlite-vec-go-bindings` version or the `sqlite_vec.Auto()` /
  `SerializeFloat32` API, check that module's README and adjust `go.mod` / the calls in
  `internal/db/store.go`.
- **FTS5 build tag.** Uses `-tags sqlite_fts5`. If your `go-sqlite3` version wants
  `-tags fts5` instead, change it in the `Makefile`.
- **Access control is minimal.** The server binds `127.0.0.1` by default and offers
  optional HTTP Basic auth (`AUTH_PASS`); there are no accounts, no per-user permissions
  and no audit trail. The one privileged action — publishing into the corpus — is gated by
  a second shared password (`BA_PASS`), which is a permission boundary, not an identity.
  To let the team in, publish through a tailnet or a Cloudflare Tunnel rather than opening
  a port — see the
  **[Deploy page](https://mega-hub-studio.github.io/mega-docs/deploy.html)**.

## Frontend assets: two manifests, one design system

Both front ends use [8-BIT NES](https://github.com/TuTranMVP/8bit-components) (**0.7.3**),
and each gets it a different way — which is the whole reason there are two manifests and
neither knows the other's versions:

| | the app (`web/ui`) | the guide (`web/*.html`) |
|---|---|---|
| how it arrives | npm, bundled by Vite into `web/dist` — committed, embedded, served from the binary | jsDelivr at runtime: there is no build step, just four static files on GitHub Pages |
| pinned in | [`web/ui/package.json`](web/ui/package.json), bytes in `package-lock.json` | [`web/vendor.sha384`](web/vendor.sha384) |
| verified by | `npm ci` against the lockfile; CI rebuilds and diffs `web/dist` | `integrity="sha384-…"`, so the browser refuses a byte that doesn't match |

**One line to upgrade the guide.** `web/vendor.sha384` is the only place its versions and
digests appear — `docsbase.html` asks for `url "8bit-nes" "all.min.css"` and Go resolves
both the URL and its `sri` from that manifest. Change the version, run `make vendor`,
done; a half-finished bump fails at **startup**, not in someone's browser. A test fails if
the version `AGENTS.md` quotes disagrees with it.

Why pin at all: the floating spec broke this page once. `marked` stopped shipping
`marked.min.js` at its package root after v4, so the old unpinned URL began 404ing and
answers silently stopped rendering.

Why the current pin is where it is —
[8bit-nes 0.7.3](changelog/2026-07-28-8bit-nes-0.7.3.md): a `<nes-toc>` rebuild bug this
project reported after measuring it on a phone, with digests verified against the
published tarball rather than the changelog, because a release has once been tagged
without the fix it claimed. Earlier releases are why this repo overrides less than it used
to — the reasoning stays in `changelog/` rather than here.

The diagrams are not part of either manifest: mermaid never reaches a browser at all. See
`make diagram` in [`CLAUDE.md`](CLAUDE.md#commands) for the rule, and the *How it works*
section of the guide for why.

### Air-gapped / no egress

Nothing to do: the app fetches nothing at runtime. Vue, the design system, marked,
DOMPurify and mermaid are npm dependencies of `web/ui`, bundled into `web/dist` and served
from the binary — content-hashed, so `/assets/` goes out `Cache-Control: immutable`. Even
the diagram renderer is a lazy *chunk* rather than a CDN fetch: it loads the first time an
answer actually contains a diagram, from the same origin.

`make vendor` still exists, and it is the **guide's**: four static pages with no build
step, loading the design system from jsDelivr with an `integrity` attribute. Vendoring
lets you render and read them with no egress at all:

```bash
make vendor                                       # fetch + sha384-verify into web/vendor/
go run ./cmd/rendocs -d /tmp/site -base /vendor   # the guide, pointing at local copies
```
