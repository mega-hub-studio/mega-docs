# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Read these first, not this file

Three documents are authoritative and maintained; this file only adds what they
don't cover.

| | What it answers |
|---|---|
| [`AGENTS.md`](AGENTS.md) | The two facts agents get wrong, and which **pinned** design-system docs to read (the docs *site* is unversioned and will describe components this repo does not have) |
| [`README.md`](README.md) | File-by-file reference, the "where do I add…?" table, the HTTP surface |
| [`changelog/`](changelog/) | Session handoffs: what is deployed where, decisions already settled, open work, and host quirks that cost an hour to find |
| <https://mega-hub-studio.github.io/mega-docs/llms.txt> | Machine index of the published guide, generated from the pages so it cannot disagree with them |

Start a task by reading the changelog entry for the deployment you are touching.
State that lives outside git is recorded there and nowhere else.

## Critical rules

Non-negotiable, and each one names what enforces it. A rule with no enforcer is a hope —
if you add one here, add its check in the same commit or mark it `prose only` honestly.

| # | Rule | Enforced by |
|---|---|---|
| 1 | `CORPUS_DIR` is the source of truth; `knowledge.db` is derived. A confirmed answer is written as a **file** first, then indexed | `TestConfirmedAnswerBecomesAFileAndThenACitation` |
| 2 | Reads are open, writes are gated. An unset `BA_PASS` means **no write surface**, not open writes | `internal/server` tests (403 unset · 401 wrong) |
| 3 | The cache signature covers everything an answer depends on — corpus, chat model, prompt hash — and the **scope** lives in the key, not the signature | `TestTheSameQuestionInAnotherScopeIsAnotherAnswer`, `TestIndexingInvalidatesTheCache` |
| 4 | A scope filters **both** retrievers before they rank, never after fusion | `TestScopedSearchRanksWithinTheScope` |
| 5 | A miss is never cached; a partial answer always is | `TestOnlyAWholeMissSkipsTheCache` |
| 6 | `cmd/ingest.docPath` and `rag.SafePath` agree: one document, one identity | `cmd/ingest` + `rag` path tests |
| 7 | The version and digest of every front-end asset live only in `web/vendor.sha384` | `TestVendorTreeMatchesTheManifest`, `TestAgentNotesPinMatchesTheManifest` |
| 8 | No credential in a tracked file | `make secrets` |
| 9 | Plumbing (`web/app/*.js`) never touches Vue | `TestPlumbingDoesNotImportVue` |
| 10 | A composable never imports another composable | `TestComposablesDoNotImportEachOther` |
| 11 | A component holds no branches — props, emits, compose, return | `TestComponentsHoldNoLogic` |
| 12 | Everything a template binds exists in the code behind it | `TestTemplatesBindNothingUndefined` |
| 13 | Go lint stays at **zero** findings | `make lint` (in `make check`, and in CI) |
| 14 | The product needs no Node and no build step | *prose only* — the day it stops being true, `make build` will tell you |

Rules 9–12 exist because the Vue 3.5 refactor made three new mistakes possible that nothing
else would catch: a template binding with no definition renders blank with no error, a
component quietly reabsorbing logic undoes the split, and one composable importing another
turns a flat set of files into a graph. All four were mutation-tested — each fails, with an
actionable message, when its rule is broken.

## Commands

Every Go command needs the build tags — `sqlite-vec` and FTS5 are cgo, and a plain
`go test ./...` compiles without FTS5 and fails at runtime. The Makefile sets
`TAGS := sqlite_fts5` and `CGO_ENABLED=1`; match it when calling `go` directly.

```bash
make deps                  # go mod tidy
make check                 # THE GATE: tests, gofmt, go vet, golangci-lint, deadcode, credential scan
make lint                  # golangci-lint alone (see .golangci.yml — every disable has a reason)
make lint-fix              # …applying what it can fix; read the diff
make lint-js               # optional: eslint + @antfu/eslint-config, installed into .cache/
make build                 # bin/knowledge + bin/ingest
make server                # run on :8080
make ingest DOCS=./docs    # index a folder (.md / .txt only)

# one test, one package
go test -tags sqlite_fts5 -run TestSafePathKeepsTheFolders ./internal/rag/
go test -tags sqlite_fts5 -count=1 ./internal/server/   # -count=1 defeats the cache
```

Two commands reach a real provider, and both read `.env` so a key never lands on a
command line:

```bash
make live    # does this provider serve both endpoints? what embedding width? does chat stream?
make smoke   # the whole product: ingest a fixture, ask, check the reply streams and cites it
```

`make live` **cannot see the repo `.env`**: `go test ./internal/ai/` runs with CWD in
the package directory and `config.Load()` reads `./.env`. Prefix it:
`set -a; . ./.env; set +a; make live`.

Run `make smoke` after touching the prompt or retrieval — the dev page names it as
the check that fails when a reply stops citing its file.

Other targets: `make switch-embed` (move embeddings to another provider: validates
the key *before* dropping the index), `make vendor` (fetch + sha384-verify CDN assets
for `ASSET_BASE=/vendor`), `make diagram` (re-render `web/*.mmd`; the SVG is committed
and `make check` fails if the source hash drifts).

## Architecture

One binary. Dependencies point one way — `cmd` → `internal` → nothing — so each layer
reads and tests on its own.

```
cmd/server    wiring only: config in, deps constructed, handler served
cmd/ingest    the indexing CLI
cmd/rendocs   renders the guide pages to static files for Pages; no cgo

internal/server  HTTP: routes, cache policy, SSE. Knows no SQLite and no templates.
internal/rag     the domain: chunk → embed → retrieve → grounded answer → QA loop
internal/ai      one OpenAI-compatible client (embeddings + chat streaming + usage)
internal/aitest  a fake provider over httptest — the whole pipeline, no key needed
internal/db      SQLite: sqlite-vec + FTS5, hybrid search with RRF, tickets, answer cache
internal/config  env → Config, with defaults
web/             Go templates + embedded ES modules; no build step
```

### The seams

`internal/server` depends on three narrow interfaces, never on the engine. Only the
first is required — the write sides are nil-able, and their routes **disappear rather
than half-work**:

- `Answerer` — ask a question, describe the corpus (required)
- `Knowledge` — the QA loop: queue, draft, confirm, reject (nil → no `/api/tickets`)
- `Importer` — document import (nil → no `POST /api/documents`)

So the entire HTTP surface is covered by `go test` with fakes: no API key, no
database. On the front end the same idea — `app.js` never sees an `AbortController`,
a `TextDecoder` or `visualViewport`.

### Five invariants worth not breaking

1. **`CORPUS_DIR` is the source of truth; `knowledge.db` is derived.** `ingest docs`
   rebuilds the database. A BA-confirmed answer is *written as a file* into
   `CORPUS_DIR/qa/ticket-N.md` and then indexed, precisely so this stays true — which
   is why the answer to "how do we back up?" is "put the documents folder in git".
2. **Reads are open; writes are gated.** `BA_PASS` guards confirming an answer,
   dismissing a ticket, and importing a document (`X-BA-Pass` header, constant-time
   compare). An unset `BA_PASS` means **no write surface at all**, not open writes —
   forgetting to configure a secret must never be how you end up without one.
   `/api/health` reports `writes` so the UI can say so before an answer is typed.
3. **The cache signature covers everything an answer depends on**: the corpus, the
   chat model, and a hash of the system prompt (`Engine.sig`). Adding a dependency an
   answer can change under — a retrieval parameter, a new prompt section — means
   adding it here, or the instance serves answers produced under rules it no longer
   has. The **scope** is the exception, and belongs in the *key* instead
   (`db.cacheKey`): a signature is pruned when it changes, so scoping it would wipe
   every other folder's answers on each pick.
4. **A scope is filtered before ranking.** `db.Search` constrains both retrievers —
   the vector KNN via `chunk_id IN (…)` and BM25 via the same subquery on `rowid` — to
   the chunks under the scope, so `TOP_K` counts matches inside it. Filtering after RRF
   fusion returns fewer sections and thins the answer without saying so. `rag.Scope`
   canonicalises the string, once, because it is half the cache key.
5. **The document path is one identity.** `cmd/ingest.docPath` and `rag.SafePath`
   must agree: relative to `CORPUS_DIR`, folders kept, `..`/absolute/hidden/`qa/`
   refused. Two spellings of one file become two documents, each cited separately.

### The schema has no migrations — on purpose

`schema.sql` is `CREATE TABLE IF NOT EXISTS` only, and it is applied on every start.
That means **a new column never reaches a database that already exists**: the table is
there, so the statement does nothing, and every query naming the column fails at
runtime on the deployed instance while passing locally against a fresh file.

There is no migration runner because there does not need to be one — invariant 1 says
the database is derived. The upgrade path for a schema change is to rebuild:

```bash
sudo systemctl stop knowledge
rm -f state/knowledge.db state/knowledge.db-wal state/knowledge.db-shm
./bin/ingest corpus            # re-embeds everything: costs one provider bill
sudo systemctl start knowledge
```

So the real cost of a column is one re-ingest, and the two ways to avoid paying it are
worth trying first: derive the value at query time, or encode it in an existing column
(the scope lives inside `answers.q_norm` for exactly this reason). Write the rebuild
into the changelog entry for any change that needs it, or the next person meets it as
a runtime error.

### Traps that have already cost time

- **The `NoAnswer` sentinel is not a substring test.** A reply that *is* the sentence
  is a miss and must not be cached; a partial answer that merely contains it (a model
  naming the part the documents don't cover) is a real answer worth caching. That is
  `isMiss`, and a prompt rule cannot replace it — models emit the sentence however
  firmly the prompt reserves it.
- **A Vue `computed` name that collides with a `data` key silently loses.** `data`
  wins and every field reads `undefined`, with no console error.
- **`cp` over a running binary fails with `Text file busy`.** Install with `mv`.
- **Numbers the UI shows must be measured, not estimated.** Token counts come from
  the provider's own usage frame; `CONTEXT_WINDOW` and `PRICE_IN`/`PRICE_OUT` are
  zero by default and the status line prints *nothing* rather than a zero — an
  unmeasured cost and a cost of nothing are different facts.

### Front end

**Vue 3.5, Composition API, no bundler.** `Vue` is a global from `index.html` (the build
that ships the compiler, so in-DOM templates work), the Go binary serves `web/app/` from
its embed FS, and there is no build step to run. **`make build` before testing the built
artifact**, or you are debugging the old bytes.

Four layers, and the rule for each is one sentence. Put a change in the lowest layer that
can hold it:

| layer | files | may contain | may not |
|---|---|---|---|
| **plumbing** | `chat.js` `qa.js` `upload.js` `answer.js` `diagram.js` `library.js` `session.js` `viewport.js` | fetch, SSE, storage, markdown, DOM maths | any Vue import — these run in a bare console |
| **logic** | `use/*.js` | reactive state and every branch | another composable's state, or markup |
| **components** | `ba.js` `tree.js` | props, emits, compose, return | branches — a component with an `if` is a composable nobody wrote yet |
| **wiring** | `app.js` | who gets what, and what the template binds | logic of its own |

One composable per concern: `conversation` (turns, streaming, persistence), `corpus`,
`scope`, `qaloop`, `runtime`, `statusline`, `diagrams`, plus `gate` (the BA password),
`importer`, `tickets`, `nestree`, `toast`.

Three rules keep the layers from leaking:

- **A composable never reaches for another's state.** What it needs arrives as an
  argument — `useConversation` gets the scope, a scroll function and an `onSettled`
  callback, not the corpus. Reactive inputs it does not own arrive as *getters*
  (`documents: () => props.documents`), so it never holds a stale array.
- **A component is a contract, not a place to work.** `ba.js` is 54 lines: four props, two
  emits, three composables, one return. Its logic moved to `use/gate.js`,
  `use/importer.js` and `use/tickets.js`, where each piece is readable alone.
- **Everything the template names must be in the returned object.** A missing key is
  `undefined` at render with no error — the same trap as a `computed` colliding with a
  `data` key in the old Options API. When you add a binding, add it to the return.

Composables read `Vue` *inside* the function (`const { ref } = Vue`), never at module
scope: the global is a classic script and a module body can evaluate before it exists.

The design system (8bit-nes) owns components; `web/app/styles.css` owns layout only.
Before writing CSS, check the pinned `llms.txt` for a recipe that already exists —
`.statusline`, `.pbar`, `.spinner`, `.datalist` and `<nes-tree>` are all there — the
tree is now in use (`web/app/tree.js`), and it renders once from a child JSON payload,
so the component replaces the element rather than mutating it. Two
selectors are defined twice in the library (`.row` is also a tree row, with
`cursor: pointer`); scope app rules rather than reusing an ambiguous class.

The binary does **not** serve the guide pages — it is the chat app and nothing else,
with one link out to the published site. Do not add doc routes to it.

## Conventions

- The version and digest of every front-end dependency live in exactly one place,
  [`web/vendor.sha384`](web/vendor.sha384), which drives both the page's `integrity`
  attributes and `make vendor`. A half-finished bump fails at **startup**, not in a
  browser.
- **Linting is configured, not inherited.** [`.golangci.yml`](.golangci.yml) starts from
  the vendored `golang-lint` skill's config and turns off five linters *with the reason
  written next to each*: whitespace rules that fight this repo's comment style, missing
  `t.Parallel()` on tests that own real files, `noctx` on a store whose queries are
  microseconds of local SQLite. The tree is at **zero findings**, so a new one is a new
  fact rather than background noise — keep it that way, and if a rule has to go, say why
  in the file.
- `make check` runs in CI on every push and pull request
  ([`.github/workflows/check.yml`](.github/workflows/check.yml)), with staticcheck and
  deadcode installed so nothing is skipped. It runs `make vendor` first, because
  `web/vendor/` is gitignored and one test asserts the tree matches every pin.
- Comments explain *why*, and name the failure that motivated the code. This repo's
  existing comments are the style guide; match their density and voice.
- Vendored Go style skills (`.agents/skills/`, `.claude/skills/`) come from
  `samber/cc-skills-golang` and are pinned in `skills-lock.json` — read them, don't
  re-add them.
