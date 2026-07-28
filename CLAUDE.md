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
| 7 | Two dependency manifests, neither knowing the other's versions: the **app's** in `web/ui/package.json` (bundled), the **docs pages'** CDN pins in `web/vendor.sha384` | `TestPinsParseRealManifest`, `TestVendorTreeMatchesTheManifest`, `TestAgentNotesPinMatchesTheManifest` |
| 8 | No credential in a tracked file | `make secrets` |
| 9 | Plumbing (`web/ui/src/lib/*.js`) never touches Vue | `TestPlumbingDoesNotImportVue` |
| 10 | A composable never imports another composable | `TestComposablesDoNotImportEachOther` |
| 11 | A component holds no branches in `<script setup>` — props, emits, compose, template | `TestComponentsHoldNoLogic` |
| 12 | Everything a template binds exists in the code behind it | `vue/no-undef-properties` — `make lint-js`, in `make check` |
| 12b | The shell wires; it does not reach for transport | `TestTheShellDoesNotReachForTransport` |
| 13 | Go lint stays at **zero** findings | `make lint` (in `make check`, and in CI) |
| 14 | `go build`, `go install` and `git pull && make build` need **no Node**: `web/dist` is built by `make ui` and committed | `TestBuiltUIMatchesItsSources` (stale bundle), plus CI rebuilds from the lockfile and diffs |
| 15 | Every feature a section documents names its routes, knobs and tests — and every `/api/` route and every variable `internal/config` reads is named by some section | `TestEverySpecNameExistsInTheCode`, `TestEveryRouteAndKnobIsSpecified` |
| 16 | `spec.json` and `llms.txt` are generated from the pages, never written by hand | `TestSpecJSONIsGeneratedFromThePages`, `TestLLMsIndexListsEveryPage` |

Rules 15–16 are what make the guide pages the **spec** rather than a description of the
code. A `<section>` that maps to code declares the mapping in its own markup:

```html
<section id="scope" data-feature="scope"
         data-api="POST /api/chat" data-env="TOP_K"
         data-test="TestScopedSearchRanksWithinTheScope">
```

`web/spec.go` collects those into the published `spec.json`; `web/spec_test.go` checks the
join **both ways**. Adding an endpoint or an environment variable therefore fails `make
check` until a section documents it — so the order of work is: write the section, declare
the join, watch it go red, then implement. The full five steps are on the Dev page
(`dev.html#spec`), which is also where an agent is pointed.

Rules 9–12 exist because the front end can break silently: a template binding with no
definition renders blank with no error, a component quietly reabsorbing logic undoes the
split, and one composable importing another turns a flat set of files into a graph. Rule 12
was a Go regex over module text until the SFCs landed; it is `vue/no-undef-properties` now,
which reads a real parse of a real component — the trigger that had been written down for
promoting ESLint into the gate.

Rule 14 changed shape rather than going away. The app is a Vite build now, and **the build
output is committed** precisely so the rule still holds where it matters: nobody needs Node
to build the binary, run it, or deploy it. Node is a *contributor's* tool — `make ui`,
`make ui-dev`, `make lint-js` — and every one of them says so when it is missing.

## Commands

Every Go command needs the build tags — `sqlite-vec` and FTS5 are cgo, and a plain
`go test ./...` compiles without FTS5 and fails at runtime. The Makefile sets
`TAGS := sqlite_fts5` and `CGO_ENABLED=1`; match it when calling `go` directly.

```bash
make deps                  # go mod tidy
make check                 # THE GATE: tests, gofmt, go vet, golangci-lint, deadcode, credential scan
make lint                  # golangci-lint alone (see .golangci.yml — every disable has a reason)
make lint-fix              # …applying what it can fix; read the diff
make lint-js               # eslint over web/ui (antfu + vue); in `check`, skipped without node_modules
make check-ui              # optional: renders the guide, serves it, measures it in Chromium
make check-wt              # optional: drives every diagram walkthrough (prev/next + highlight)
make ui                    # build the app's front end (Vite) into web/dist — commit the output
make ui-dev                # Vite dev server on :5173 with HMR, /api proxied to :8080
make build                 # bin/knowledge + bin/ingest (no Node: it uses the committed web/dist)
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

A diagram gets a **walkthrough** by writing markup, not script: wrap the inlined SVG in
`<nes-focus-svg id="X">`, author the steps as `<div id="X-steps">` with
`data-title`/`data-title-vi`/`data-focus` children, and add `<nes-walkthrough for="X">`.
`docsbase.html` folds them into the component's step JSON for every diagram on every page —
in a *classic* inline script, because `<nes-walkthrough>` reads its steps once at upgrade
time and `elements.min.js` is a module (so it runs later). `data-focus` is a `|`-separated
list matched against node text, case-insensitively. With JS off, every step's prose is
still on the page in both languages.

Other targets: `make switch-embed` (move embeddings to another provider: validates
the key *before* dropping the index), `make vendor` (fetch + sha384-verify the docs pages'
CDN assets, for `rendocs -base /vendor`), `make diagram` (re-render `web/*.mmd`; the SVG is committed
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
web/ui           the app's front end: Vue 3.5 SFCs, JavaScript, built by Vite
web/dist         that build — committed, embedded, served. `make ui` regenerates it
web/*.html       the guide: Go templates, both languages inline, no build step at all
                 spec.go → spec.json: the pages' own annotations, machine-readable
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
- **`.steps` in 8bit-nes is a stage bar, not an instruction list.** It is
  `display: inline-flex` with mono uppercase `<li>`s, for "STAGE 1 · 2 · 3". Used for a
  numbered list with paragraphs it renders as uppercase columns four characters wide on a
  phone — which shipped. Numbered instructions on the docs pages are `.step` blocks;
  `make check-ui` fails on `main .steps > li`.
- **`nes-toc` sets `z-index: 20` on itself.** The sticky header has to sit above it or a
  popup opened from the header (the section finder) renders behind the index bar: visible,
  untappable, and no error anywhere. A popup's own z-index cannot help — it is inside the
  header's stacking context.
- **`make secrets` only sees *tracked* files.** It is `git grep`, so running the gate
  before `git add` scans a smaller tree than CI does — a new generated file can pass locally
  and fail on the first push. That happened with `web/ui/package-lock.json` (every entry
  carries a sha512 integrity string) and `web/dist` (minified third-party code matching by
  accident). Both are excluded now, with the reason next to them.
- **`go ./...` walks into `web/ui/node_modules`.** One npm dependency (flatted) ships a Go
  package, and the linter reported seven findings from somebody else's code. The Go tool has
  no directory ignore, so the Makefile spells the packages out: `PKGS := ./cmd/... ./internal/... ./web`.
- **A stale `golangci-lint` on PATH makes the gate lie.** CI installs `@latest`; an
  older binary installed locally reports zero while CI fails (it happened on `goconst`
  and a new `gosec` rule). `make lint` prints the version it used — compare it with the
  one in [`check.yml`](.github/workflows/check.yml) before trusting a green run.
- **Numbers the UI shows must be measured, not estimated.** Token counts come from
  the provider's own usage frame; `CONTEXT_WINDOW` and `PRICE_IN`/`PRICE_OUT` are
  zero by default and the status line prints *nothing* rather than a zero — an
  unmeasured cost and a cost of nothing are different facts.

### Front end

**Vue 3.5 single-file components, built by Vite from `web/ui`.** JavaScript, not
TypeScript — one maintainer, and the type surface is three API shapes `spec.json` already
describes. The output lands in `web/dist`, which is **committed and embedded**: that is
what keeps `go build` and a deploy free of Node, and `TestBuiltUIMatchesItsSources` is
what keeps it honest. **`make ui` before testing the built binary**, or you are debugging
last week's bundle.

Day to day: `make server` in one shell, `make ui-dev` in another — Vite on :5173 with HMR,
`/api` proxied to :8080, so an SFC edit shows up without a build and still talks to the
real engine.

Four layers, and the rule for each is one sentence. Put a change in the lowest layer that
can hold it:

| layer | files | may contain | may not |
|---|---|---|---|
| **plumbing** | `src/lib/` — `chat.js` `qa.js` `upload.js` `answer.js` `diagram.js` `library.js` `session.js` `viewport.js` | fetch, SSE, storage, markdown, DOM maths | any Vue import — these run in a bare console |
| **logic** | `src/composables/*.js` | reactive state and every branch | another composable's state, or markup |
| **components** | `src/components/*.vue` | props, emits, compose, template | branches in `<script setup>` — a component with an `if` is a composable nobody wrote yet |
| **wiring** | `src/App.vue`, `src/main.js` | who gets what, and what the template binds | logic of its own, or a reach into `src/lib/` (except `viewport.js`: it binds the dock element the shell owns) |

One composable per concern: `conversation` (turns, streaming, persistence), `corpus`,
`scope`, `qaloop`, `runtime`, `statusline`, `diagrams`, plus `gate` (the BA password),
`importer`, `tickets`, `nestree`.

Three rules keep the layers from leaking:

- **A composable never reaches for another's state.** What it needs arrives as an
  argument — `useConversation` gets the scope, a scroll function and an `onSettled`
  callback, not the corpus. Reactive inputs it does not own arrive as *getters*
  (`documents: () => props.documents`), so it never holds a stale array.
- **A component is a contract, not a place to work.** `BaScreen.vue` declares four props
  and two emits, composes the gate and the tickets, and delegates the importer to
  `ImportPanel.vue` — because a composable belongs to whoever renders its state.
- **The shell destructures.** `const { turns, ask } = useConversation(…)`, so the template
  names plain values and Vue unwraps them. `chat.turns.value` in a template is the smell
  that the wiring has started keeping state of its own.

Things that were true before the bundler and are not now: `Vue` is an import, not a
global; `toast` and `setMute` are imported from `8bit-nes` rather than injected; mermaid
arrives as a dynamic `import("mermaid")` that Rollup gives its own chunk (never import it
statically — that moves 3.4 MB into first paint and only the chunk sizes would say so);
and `<nes-*>` is declared a custom element in `vite.config.js`, at *compile* time, because
the runtime flag is too late for a pre-compiled template.

On the **docs pages** (`web/docsbase.html`) spacing is one rule, not per-recipe margins:
`--flow` for every block that follows another, `--sp-5` above an `h3`, `--sp-7` between
sections. Before that there were four different gaps and 31 pairs of blocks touching at
0px on one phone screen. A table row is a block below 40rem — the column headings are
copied onto the cells by the foot script — because at 390px a three-column grid gives each
value about four characters. `make check-ui` measures all of it.

The design system (8bit-nes) owns components; `web/ui/src/styles.css` owns layout only.
Before writing CSS, check the pinned `llms.txt` for a recipe that already exists —
`.statusline`, `.pbar`, `.spinner`, `.datalist` and `<nes-tree>` are all there — the
tree is in use (`components/CorpusTree.vue` + `composables/nestree.js`), and it renders once
from a child JSON payload,
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
