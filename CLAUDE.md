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
| 1 | **The Knowledge DB is the source of truth.** A document — imported, written or confirmed — is a row with its body, and the WebUI is the only way one enters. Nothing writes a corpus file | `TestConfirmedAnswerBecomesADocumentAndThenACitation`, `TestARemovedDocumentStopsAnsweringAndItsTextSurvives` |
| 2 | Reads are open, writes are gated. An unset `BA_PASS` means **no write surface**, not open writes | `internal/server` tests (403 unset · 401 wrong) |
| 3 | The cache signature covers everything an answer depends on — corpus and prompt hash — while the **scope and the chat model** live in the key, not the signature | `TestTheSameQuestionInAnotherScopeIsAnotherAnswer`, `TestAnotherModelIsAnotherAnswerAndBothSurvive`, `TestIndexingInvalidatesTheCache` |
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
| 17 | **KISS, taken to the extreme: the smallest correct change, and a second copy of a fact is a bug.** Delete before you add | `make dead` (unreachable from any binary) · `make lint` (`unused`, `goconst`) · the rest is `prose only` |
| 18 | **Four root documents, one job each.** A fifth is a parallel truth | `TestRootDocsAreTheFourWeKnowAbout` |
| 19 | `README-MEGA-DOCS.md` is the **vNext brief, not the spec.** Code disagreeing with it is a gap with a recorded decision, never a bug to fix on sight | `TestRootDocsAreTheFourWeKnowAbout` (the join must stay in README.md) |
| 20 | **No overhead, no over-engineering.** No abstraction with one caller, no knob nobody turns, no layer for a future that has not arrived. The cheapest correct thing wins | `make dead` · `make lint` (`unused`) · `TestEveryRouteAndKnobIsSpecified` (a new knob must earn a documented section) · the rest is `prose only` |
| 21 | **No new test file, no unit/E2E scaffold for a change.** Extend the test that already owns the rule; verify against the running product | `prose only` — `make smoke` and `make live` are the verification of record |
| 22 | **Complexity hides behind one seam; the call site reads as intent.** Modern idiom, the plainest syntax that is correct, no ceremony, no comment that restates its code — and a name a *grep* resolves, because an agent infers from what it can find | `make lint` (`gocyclo` 16 · `nestif` · `funlen` · `gocritic` · `intrange` · `usestdlibvars` · `godot`) · `make lint-js` (`no-var`, `prefer-const`, `prefer-template`, 22 `unicorn/*`, 15 `jsdoc/*` — all at `--max-warnings 0`) |
| 23 | **Read the layer's vendored skill before writing in it**, `ponytail` first on any coding task. They are the style source; *Skills* below records the routing, the precedence and every place this repo differs | `TestVendoredSkillsMatchTheirRouting` (vendored ⇔ symlinked ⇔ recorded ⇔ routed); `.golangci.yml` and `web/ui/eslint.config.js` are the parts already machine-checked. What a skill *says* is `prose only` |
| 24 | **HARD: no technical debt leaves a change.** No deferred marker, no suppressed finding, no half-migration, no stale doc — a change lands whole and `make check-full` green, or it does not land | `godox` + `no-warning-comments` (a deferred-work marker is a lint error, both languages) · `nolintlint` (a suppression must name the linter *and* the reason) · `make check-full` · rule 13's **zero** findings |
| 25 | **The version is a git tag; the changelog is generated from `git log`.** `make release V=vX.Y.Z` is the only thing that changes either — never a `VERSION` file, never a hand-edited `web/release.json`, never a `CHANGELOG.md`. No tag means an **empty** version, which removes the badge rather than asserting a stale number | `TestReleaseNotesAreGenerated` (the do-not-edit marker · the tag shape · a sha behind every line) · `TestEveryRouteAndKnobIsSpecified` (`GET /api/release` must earn a documented section) · `TestRootDocsAreTheFourWeKnowAbout` (a root `CHANGELOG.md` is a fifth doc) |
| 26 | **HARD: the official guide syncs in the same commit, and the claim it replaces is retired.** A behaviour change edits its `<section>`; the superseded passage is **deleted**, never parked beside its replacement, and its dead sentence is added to `retiredClaims` so it cannot return. Stale prose is context an agent loads and believes | `TestGuidePagesCarryNoRetiredClaim` (every published page, both languages) · `TestEverySpecNameExistsInTheCode` · `TestEveryRouteAndKnobIsSpecified` · `TestSpecJSONIsGeneratedFromThePages` · `make check-ui` |
| 28 | **HARD: the phone is the design, not a breakpoint. Base styles are 390px; one `min-width` query upgrades to desktop, never a `max-width` query down.** A layout decision is made at 390 first and widened only if it survives there — and the page may never scroll sideways, at any width | `make check-ui` measures every guide page at **390 · 1440 · 1920** and fails on a section wider than its parent · the app is `prose only`, and the two cheap probes plus the overflow this caught are in `changelog/2026-07-30-mobile-first.md` |
| 27 | **HARD: a seam belongs to the container, never to the blocks. One `gap`, set once on the parent — no margin per child, and no two stacked blocks touching at 0px.** A design-system recipe spaces only *inside* itself; the space *between* two of them is this repo's job, and no app container may stack blocks without declaring it | `make check-ui` (the guide's `--flow` rule) · the app is `prose only` — measure it in the running product at 390×844, and the probe plus the five pairs it caught are in `changelog/2026-07-30-seams.md` |

Rule 28 was the practice before it was a rule, which is exactly why it needed writing down:
`styles.css` has said "base styles are the phone — never the reverse" since it was written, and
its comments carry two dozen measurements taken at 320 · 360 · 390. None of that was in the
rules, so nothing stopped a change being *reasoned* about at laptop width and measured after.

Two directions it fixes, and they are not the same:

1. **Order.** Decide at 390, then widen. The reverse produces a layout that technically reflows
   and is unusable: the bar was a quarter of an iPhone 14's screen until it was measured there,
   and `.steps` renders as four-character columns on a phone — a trap this file keeps because it
   *shipped*.
2. **The one absolute.** No sideways scroll, ever. Everything else is a judgement about spacing;
   this is the only measurement with a right answer, and it is `documentElement.scrollWidth ===
   innerWidth`. A block too wide for a phone must scroll or pan **inside itself** — the way a
   `<pre>`, a `.table-wrap` and `<nes-zoom>` all do — or be capped.

The trap that motivated the rule is the second one wearing the first one's clothes. 8bit-nes opts
a list of elements out of `--prose-measure` with `max-inline-size: none`, on the stated principle
*"if the content is the width, it opts out"* — correct for every entry that scrolls itself, and
wrong for `img`, which has no such escape. Unwrapping a lone image so that opt-out applied gave a
1400px screenshot `max-inline-size: none`, and it rendered **1404px inside a 1207px card**: a
reading limit and a container limit had been treated as one cap. `.prose > img` takes the
container limit back (`styles.css`), and the distinction is the rule in one line — an image may
escape the *measure*, nothing may escape the *container*.

What is machine-checked and what is not: the guide is measured at three widths on every page, so
a section that outgrows its parent fails `make check-ui`. **The app is not measured by anything**,
and rule 27 already says so for the same reason — a rig for one screen is what rule 21 refuses.
Two probes cost nothing and catch the whole class; run them against the running product before
saying a layout is done:

- `documentElement.scrollWidth === innerWidth` — the absolute above.
- every child of the answer card reports **one** `getBoundingClientRect().left`. Five blocks that
  share an edge read as a column; one block 2px off reads as a mistake nobody can name. Measured
  at 390: all five agree at 34px, and so does every block in `.prose`.

Rule 27 is the layout half of rule 17, and it is a rule because the same defect shipped **five
times** in one card family before anyone measured it. The cause is one fact about the design
system that is easy to read the wrong way: **a recipe spaces its own parts and nothing else.**
`.field` puts --sp-2 between a label, its control and its hint. `.card > .head` carries a
`margin-block-end`. `.sources` and `.feedback` carry a `margin-block-start`. `.callout`, a
*text* recipe, carries nothing. So a `.card` — **not** a flex container — stacked eight blocks
whose seams were whatever each one happened to bring, and where that was nothing it was zero:
a hint against the next field's label, a ticket badge against the actions row, the Find field
against the document list, that list against the form, a password field against its button.

What the rule forbids is not the 0px, it is the *per-child margin* that produces it. Three
cards had been fixed one at a time before this, each carrying its own copy of
`display: flex; flex-direction: column; gap: …` — so the fix was written three times and the
fourth card still had the bug, which is rule 17's drift on schedule. It is one declaration now
(`main .card` in `web/ui/src/styles.css`), and a card names **only its own value**: --sp-3 for
a queue that is scanned, --sp-5 for a form that is typed into.

Two consequences, both got wrong at least once:

1. **Give the recipe's own margin back.** A surviving `margin-block-start` lands *on top of*
   the gap, so that one seam measures half again as much as every other seam in the same card
   — which reads as "the head is detached" and gets fixed by nudging something unrelated.
2. **0px is sometimes the recipe.** `.stat`'s number and its label, `.result`'s title and its
   path: two lines of one thing, not two blocks. The rule is about blocks a reader parses
   separately, and the probe's output is where to tell them apart — whatever is left in it
   after a fix must be a library recipe's internals, or the fix is not finished.

`prose only` is honest rather than convenient. `make check-ui` measures the *guide* — that is
where `--flow` and its 31 touching pairs come from — and measuring the app needs a running
server, a real answer and a browser, which is a rig for one thing and rule 21 refuses it. What
replaces it is the probe in that changelog entry: twenty lines pasted into `pinchtab eval`,
printing every adjacent pair under 3px. Run it on both screens after touching layout.

Rule 26 is the enforcer rule 24 was missing. "No stale doc" was already written down, and
this repo still published a page telling operators that `rm knowledge.db` was safe — three
commits after the database became the corpus — with `make check` green the whole time. Every
machine-checked join is about *names*: `TestEveryRouteAndKnobIsSpecified` sees a route,
`TestEverySpecNameExistsInTheCode` sees a test name, `make check-ui` measures pixels. None of
them can tell that a correctly spelled paragraph describes last week's architecture.

So the guide is part of the change, not the follow-up to it:

1. **Same commit.** A change to behaviour edits the `<section>` that documents it. The guide
   *is* the spec (rules 15–16) and it ships to a public domain on its own cadence, so a page
   left behind is a lie with a URL.
2. **Retire the claim you replaced.** Delete the superseded passage — never leave it beside
   its replacement, which is how a page ends up teaching two architectures and a reader picks
   the wrong one. Then add the dead sentence to `retiredClaims` in `web/embed_test.go`, one
   line, so it can never come back: not by a revert, not by a copied paragraph, and not by an
   agent that read an old page and helpfully restored it.
3. **Cleanup is deletion, not accumulation.** Superseded prose, a `.env` key nothing reads, a
   changelog paragraph restating `README.md` — all of it is context an agent loads and reasons
   from. Rule 17 says a second copy of a fact is a bug; a *stale* copy is worse, because it
   reads as current.

The order that keeps it honest is rule 15's: edit the section, watch the join go red,
implement. Point 2 is what makes point 1 checkable — and the cost of inverting a decision is
one line in a list.

Rule 25 is rule 17 aimed at the one fact this repo is most tempted to keep twice. Three
places could hold "what changed" — a `VERSION` file, a `CHANGELOG.md`, and the git log — and
two of them are copies with a delay on them. So the tag is the input and everything else is
generated: `scripts/release.sh` reads `git log` between the previous tag and `HEAD`, writes
`web/release.json`, commits it, then tags **that** commit (in that order, or every deploy
reads one commit past its own release).

Its two rejections are already recorded, and re-deriving them costs a day:
`changelog/2026-07-28-deploy-and-version.md` rejected a `VERSION` file and `-ldflags -X`
because *a version somebody has to remember to bump is a version that lies*, and
`changelog/README.md` refuses release notes on the grounds that *the git log already is
one*. A tag does not repeat the first mistake, it defeats it — forget to cut one and
`ReleaseInfo().Version` is empty, so `GET /api/release` is never registered and the badge
falls back to the commit sha. Nothing on screen claims a release that was not cut.

What makes it enforceable rather than a wish: **every note names its short sha.** Prose a
human types has no commit to point at, so `TestReleaseNotesAreGenerated` cannot be satisfied
by a hand-written file — which is the check that turns "generated, not written" from a
convention into a red test. The commit in `/api/health` stays beside the tag and neither
replaces the other: the commit says which bytes are running, the tag says what changed.

Rules 17 and 20 are the two to apply before the rest, because most of the others exist to
stop a second copy of something or a part nobody needed. Rule 17 taken literally, in the
order to try them:

1. **Delete it.** Dead config, a knob nobody turns, a paragraph that restates the paragraph
   above. `.env` used to set nine keys to their own defaults from `internal/config` — nine
   numbers with two homes, and `ASSET_BASE`, which the code had already stopped reading.
2. **Point at it.** A fact belongs to one file; everything else links. `changelog/` owns a
   decision, `README.md` the reference, a guide `<section>` a feature.
3. **Only then write it**, and write the smallest thing that is correct.

The failure it prevents is not verbosity, it is *drift*: two copies of one fact are one
copy plus a lie with a delay on it, and an agent that reads both loads the lie too.

Rule 20 is its twin aimed at time rather than copies: 17 stops a fact being written
**twice**, 20 stops it being written **early**. Three questions before adding any structure
that is not the code doing the job — *who calls it twice?* (an interface with one
implementation is indirection charging rent; the three seams in `internal/server` earn
theirs, each has a real fake behind it), *who turns it?* (a knob sitting at its own default,
and a new one costs a documented section before `make check` goes green), *what breaks today
without it?* ("we'll need it when…" pays now for a benefit dated later). It is mostly
`prose only` on purpose: an over-built abstraction that is wired up is *reachable*, so
`make dead` sees nothing wrong with it and only the reviewer does.

Rule 21 is 20 aimed at the tests. The enforcer column above is a **closed set** — every
rule already names the file that owns it, so an assertion extends that file, while a 21st
test file, a fixture server, a mock of a fake or a browser rig for one button each cost more
to keep than the bug they would catch. What replaces them is the running product; which
command proves what is in *Commands* below. The line this does not cross: an invariant still
needs its enforcer, or rule 15's red-first order and CI stop meaning anything.

Rule 22 is where those meet the syntax. Complexity is not deleted, it is **placed**: one
seam holds it and the caller reads as intent — `const { turns, ask } = useConversation(…)`,
with the SSE parse and the `AbortController` nowhere near the shell. So the question is
*which layer can hold the mess* (see *Front end*), never whether to add a wrapper. Then the
plainest modern syntax, and neither half is taste — both are lint findings: Go 1.26 idiom
(`for i := range n`, `slices`/`maps`, `strings.Cut`, the `SplitSeq`/`FieldsSeq` iterators
over their slice-allocating twins, `errors.Is` and `%w`), ESM JavaScript
with `const`, `?.`/`??` and no `var`, early return over `else`, and a named function — never
a comment apologising — when `gocyclo`, `nestif` or `funlen` push back.

Rule 24 is the hard one, and it is hard because debt in this repo is not a backlog item —
it is a **lie in the gate**. A suppressed finding makes rule 13's zero mean "zero minus the
ones we hid", a deferred marker makes the code claim a plan nobody owns, a half-migration
makes `schema.sql` disagree with a deployed database, and a doc left behind makes an agent
read the lie (rule 17). So both markers are lint errors now — `godox` in Go,
`no-warning-comments` in `web/ui`, each free because this tree has none — and a `//nolint`
must name its linter *and* its reason (`nolintlint`); the four in the tree do. What a
deferred note would have said goes in `changelog/`, dated, next to the decision, or gets
done in the same change. Done means `make check-full` green, and reading its *skipped*
lines rather than assuming a green run covered them.

The second reader is an **agent**, which infers only from what it can find: one obvious place
per thing, a name carrying the fact (`isMiss`, `SafePath`, `cacheKey`), no alias, no
re-export chain, no barrel file. A symbol `grep` resolves in one hop costs one read; behind
two indirections it costs three and gets guessed instead. Rule 23 is that economy applied to
the style itself — the conventions are already written and pinned, so read the row in
*Conventions* rather than re-deriving them.

Rules 18–19 are rule 17 pointed at the documentation, and they are what keeps an agent's
context clean — four files, no overlap, so reading the tree costs four reads and returns one
answer per question:

| file | owns | must not contain |
|---|---|---|
| `README.md` | the reference: file-by-file, "where do I add…?", the HTTP surface, the Now vs vNext join | rules, or a decision's history |
| `CLAUDE.md` | these rules, the commands, the architecture and the traps | a feature description, or anything a test already asserts in prose form |
| `AGENTS.md` | the two facts agents get wrong, and the **pinned** design-system docs | anything not aimed at an agent |
| `README-MEGA-DOCS.md` | the vNext brief — the product this is becoming | any claim about what the code does today |

The join between the last two is `README.md`'s **Now vs vNext** table: shipped, next, or
blocked-and-on-what, with the decision in `changelog/`. Read it before implementing anything
from the brief — three of its lines are settled decisions *against* the brief
(`2026-07-28-vnext-collisions.md`, `2026-07-28-sot-decision.md`), and re-deriving them costs
a day and lands on the wrong answer.

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
/ship                      # the whole post-implement chain, and the one to reach for: free the
#                            rig's port → `make lint-fix` → `make check-full` → review the diff
#                            with `ponytail` at full, through rules 17/20/21/22/24 and the
#                            four-layer table. Stops at the first red step, so the reviewer
#                            never reads code the gate rejected. Report-only: applying would
#                            re-dirty the tree and void the green run. `.claude/commands/ship.md`
make check-full            # THE FINAL GATE — run before saying anything is done: ui → check
#                            → build → check-ui → check-wt, cheapest stage first
make check                 # what CI gates on, and what you run while working: tests, go vet,
#                            golangci-lint (gofmt included), eslint, deadcode, credential scan
make lint                  # golangci-lint alone, at the pinned GOLANGCI_VERSION — installing
#                            it first if PATH has another one (see .golangci.yml for why each
#                            linter is on, and which are off with the reason)
make lint-fix              # …applying what it can fix, BOTH languages. Read the diff. Never a
#                            step of `check` — the Makefile says why, next to it
make lint-js               # eslint over web/ui (antfu + vue); skipped without node_modules
make check-ui              # optional: renders the guide, serves it, measures it in Chromium
make check-wt              # optional: drives every diagram walkthrough (prev/next + highlight)
#   both are driven by PinchTab: `npm i -g pinchtab`. They skip when it is not on PATH and
#   fail, naming both causes, when it is there and starts no instance — nothing subtler is
#   possible, because every pinchtab subcommand exits 0, an unknown one included. Each run
#   starts its own instance on its own port and stops it after: PinchTab commands act on an
#   instance's current tab, so sharing one with an editor or an MCP integration makes the
#   measurements flaky (2 of 3 runs, measured). Two files know a CLI is involved and no
#   others: scripts/guide-rig.sh renders, serves and owns the browser for both checks — the
#   wrappers are four variables each — and scripts/pinchtab.mjs drives it.
make ui                    # build the app's front end (Vite) into web/dist — commit the output
make ui-dev                # Vite dev server on :5179 with HMR, /api proxied to :8080
make build                 # bin/knowledge + bin/ingest (no Node: it uses the committed web/dist)
make deploy                # from ANY tree on the machine: deploys DEPLOY_DIR (/opt/knowledge,
#                            named on the first line) — unit exists? → pull --ff-only →
#                            stale-bundle check → build → restart → health. The unit is asked
#                            first: a typo'd UNIT otherwise half-deploys (new binary on disk,
#                            old process serving a deleted inode). It says so when this tree holds
#                            commits origin does not. Never a rebase, never a push;
#                            UNIT/PORT/DEPLOY_DIR override it. `deploy-here` is the work,
#                            and it assumes it already *is* the supervisor's checkout
make release V=v0.13.0     # cut a release: generate web/release.json from git log, commit
#                            it, tag that commit. The tag is the only input (rule 25); push
#                            with `git push origin main --follow-tags`
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

Other targets: `make vendor` (fetch + sha384-verify the docs pages'
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
database. On the front end the same idea — `App.vue` never sees an `AbortController`,
a `TextDecoder` or `visualViewport`.

### Five invariants worth not breaking

1. **The Knowledge DB is the source of truth, and the WebUI is the only way in.** A
   document is a row: `documents.body` holds its text and four attributes hold what a BA
   files it under (title · alias · kind · description). A BA-confirmed answer is the same
   thing — written straight to a row at `qa/ticket-N.md`, which stays a `.md` path because
   that is what a citation prints and a scope matches, not because a file exists.
   Nothing writes to a corpus directory; `internal/rag` touches no disk at all. `ingest`
   still reads files, but as an operator's import client, and `CORPUS_DIR` is only the
   folder it reads.
   Losing the database loses the corpus, so **one thing protects it**:
   `scripts/backup.sh` — a `sqlite3 .backup` of `DB_PATH`, integrity-checked before it is
   published, written outside this machine's disk, nightly by timer or `make backup` by hand.
   The other net is that `Remove` is soft — the chunks go, so the document stops answering
   immediately, and the row keeps its text with a `deleted_at`. That is `.trash/` as a
   column, recoverable by whoever has the database.
2. **Reads are open; writes are gated.** `BA_PASS` guards confirming an answer,
   dismissing a ticket, and importing a document (`X-BA-Pass` header, constant-time
   compare). An unset `BA_PASS` means **no write surface at all**, not open writes —
   forgetting to configure a secret must never be how you end up without one.
   `/api/health` reports `writes` so the UI can say so before an answer is typed.
3. **The cache signature covers everything an answer depends on**: the corpus and a hash
   of the system prompt (`Engine.sig`). Adding a dependency an
   answer can change under — a retrieval parameter, a new prompt section — means
   adding it here, or the instance serves answers produced under rules it no longer
   has. The **scope and the model** are the exceptions, and belong in the *key* instead
   (`db.cacheKey`): a signature is pruned when it changes, so putting either there would wipe
   every other folder's — or every other model's — answers on each pick. The chat model was in
   the signature until the picker existed, where one instance had one model and the difference
   could not show.
4. **A scope is filtered before ranking.** `db.Search` constrains both retrievers —
   the vector KNN via `chunk_id IN (…)` and BM25 via the same subquery on `rowid` — to
   the chunks under the scope, so `TOP_K` counts matches inside it. Filtering after RRF
   fusion returns fewer sections and thins the answer without saying so. `rag.Scope`
   canonicalises the string, once, because it is half the cache key.
5. **The document path is one identity, and it carries the folder.** `cmd/ingest.docPath`
   and `rag.SafePath` must agree: folders kept, `..`/absolute/hidden refused, and `qa/`
   refused for anything being *created* there. Two spellings of one document become two
   documents, each cited separately. This is also why no `folder` column exists — the path
   is the scope prefix (invariant 4) and the citation name, so a second home for the folder
   would disagree with it the first time somebody renamed one. `SafePath` is the create
   rule; `readPath` is the same rules minus the `qa/` refusal, because correcting a
   confirmed answer where it already lives is an edit, not a fabrication.

### New tables come from schema.sql; new columns need a migration

`schema.sql` is `CREATE TABLE IF NOT EXISTS` only, applied on every start, so **a new
column never reaches a database that already has the table**: the statement finds the
table and does nothing, and every query naming the column fails at runtime on the
deployed instance while passing locally against a fresh file.

That was survivable for exactly as long as the database was *derived* — the upgrade path
was `rm knowledge.db && ingest corpus`, one provider bill. **It is not derived any more.**
The documents live here, so a change that cannot reach an existing database is a change
that cannot ship, and `internal/db/migrate.go` is what makes one able to. It shipped
*before* the inversion, deliberately: doing it the other way round removes the way back.

It is small on purpose — forward only, one transaction per migration, and a
`schema_version` **table** rather than `user_version`, because the first question anyone
asks an odd-behaving database is which migrations it has actually seen. Read the file's own
header before adding one; the rules that matter are: never renumber an `id`, and never edit
the SQL of a migration that has shipped.

Still worth trying before paying for a column at all: derive the value at query time, or
encode it in an existing one (the scope lives inside `answers.q_norm` for exactly this
reason).

**The rebuild is gone as an escape hatch.** `rm knowledge.db && ingest corpus` used to fix
any schema mistake for the price of one provider bill; there is nothing to rebuild *from*
now. What there is instead is last night's copy — so a bad migration costs the writes since
it, not the corpus, and `make backup` before one is the whole drill. That is why the runner
is still forward-only, one transaction per step, and boring: a restore is a recovery, not a
plan.

### Traps that have already cost time

- **The `NoAnswer` sentinel is not a substring test.** A reply that *is* the sentence
  is a miss and must not be cached; a partial answer that merely contains it (a model
  naming the part the documents don't cover) is a real answer worth caching. That is
  `isMiss`, and a prompt rule cannot replace it — models emit the sentence however
  firmly the prompt reserves it.
- **A Vue `computed` name that collides with a `data` key silently loses.** `data`
  wins and every field reads `undefined`, with no console error.
- **`cp` over a running binary fails with `Text file busy`.** Install with `mv`.
- **`.steps` in 8bit-nes is a stage bar, not an instruction list.** The list is
  `display: flex` and each `<li>` is `inline-flex`, mono uppercase, for "STAGE 1 · 2 · 3"
  (re-measured on 0.15.0 — this said `inline-flex` on the list itself). Used for a
  numbered list with paragraphs it renders as uppercase columns four characters wide on a
  phone — which shipped. Numbered instructions on the docs pages are `.step` blocks;
  `make check-ui` fails on `main .steps > li`.
- **`.pbar`'s `--fill` does not inherit, so it goes on the `<i>`, never on the bar.** 0.15.0
  registers `@property --fill { syntax: "<percentage>"; inherits: false; initial-value: 0% }`,
  while the recipe's own CSS writes `.pbar { --fill: 0% }` and the `<i>` reads
  `inline-size: var(--fill)`. Follow the container and the child resolves the registered
  *initial* instead — measured 0px against 1031.89px at the same 66.66%. The bar is empty at
  every count, at every width, with no error anywhere and the right number in the sentence
  beside it. A registered custom property is the one case where "set it on the parent" is not
  a style preference; check for `@property` before assuming a variable reaches a child.
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

Day to day: `make server` in one shell, `make ui-dev` in another — Vite on :5179 with HMR,
`/api` proxied to :8080, so an SFC edit shows up without a build and still talks to the
real engine.

Four layers, and the rule for each is one sentence. Put a change in the lowest layer that
can hold it:

| layer | files | may contain | may not |
|---|---|---|---|
| **plumbing** | `src/lib/` — `chat.js` `qa.js` `upload.js` `answer.js` `diagram.js` `library.js` `session.js` `viewport.js` | fetch, SSE, storage, markdown, DOM maths | any Vue import — these run in a bare console |
| **logic** | `src/composables/*.js` | reactive state and every branch | another composable's state, or markup |
| **components** | `src/components/*.vue` | props, emits, compose, template | branches in `<script setup>` — a component with an `if` is a composable nobody wrote yet |
| **wiring** | `src/App.vue`, `src/main.js`, `src/router.js` | who gets what, which screen, and what the template binds | logic of its own, or a reach into `src/lib/` (except `viewport.js`: it binds the dock element the shell owns) |

One composable per concern: `conversation` (turns, streaming, persistence), `corpus`,
`scope`, `qaloop`, `runtime`, `statusline`, `diagrams`, `library` (the BA's documents and
the form that writes them), plus `gate` (the BA password),
`importer`, `tickets`, `nestree`, `finder`, `lang` (i18n — the only door to vue-i18n).

Three rules keep the layers from leaking:

- **A composable never reaches for another's state.** What it needs arrives as an
  argument — `useConversation` gets the scope, a scroll function and an `onSettled`
  callback, not the corpus. Reactive inputs it does not own arrive as *getters*
  (`documents: () => props.documents`), so it never holds a stale array.
- **A component is a contract, not a place to work.** `BaScreen.vue` declares four props
  and one emit, composes the gate and the tickets, and delegates the importer to
  `ImportPanel.vue` — because a composable belongs to whoever renders its state.
- **The shell destructures.** `const { turns, ask } = useConversation(…)`, so the template
  names plain values and Vue unwraps them. `chat.turns.value` in a template is the smell
  that the wiring has started keeping state of its own.

**Which screen is showing lives in the address, not in a ref.** `src/router.js` is
vue-router over the two screens — `/#/ask` (`AskScreen.vue`) and `/#/ba` (`BaScreen.vue`,
a dynamic import, so a DEV never downloads the queue) — and it replaced a `mode` ref
mirrored into `localStorage`, which could not be linked or backed out of. Read that file's
header before changing routing: the hash is load-bearing (the binary serves one HTML file
and `TestGuideRoutesAreNotServed` asserts every other path 404s, which is a clean-URL
fallback inverted), and `/` resolving to the last screen is the one thing still stored.
The state stays in the shell rather than moving into the route components, because an
answer has to keep streaming while a BA reads the queue — so the router picks *which*
screen and `App.vue`'s template picks what each is handed.

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

The binary does **not** serve the guide pages, and does not link out to them — it is the
chat app and nothing else. Do not add doc routes to it, and do not reintroduce a
`SITE_URL`: the guide ships from this repository to Pages on its own cadence, so an
address held by a running server is a second home for a fact that already has one in
`cmd/rendocs -site`.

## Conventions

- The version and digest of every front-end dependency live in exactly one place,
  [`web/vendor.sha384`](web/vendor.sha384), which drives both the page's `integrity`
  attributes and `make vendor`. A half-finished bump fails at **startup**, not in a
  browser.
- **Linting is configured, not inherited.** [`.golangci.yml`](.golangci.yml) starts from
  the vendored `golang-lint` skill's config, and its `linters.default` is **`none`** — the
  `enable:` list is therefore the complete set, with nothing arriving from a default you
  have to look up or from an upgrade. Everything the recommended config ships and this
  repo does not run is a **comment** naming the reason (whitespace rules that fight this
  repo's comment style, missing `t.Parallel()` on tests that own real files, `noctx` on a
  store whose queries are microseconds of local SQLite) rather than a `disable:` key,
  because with `default: none` a `disable:` entry suppresses nothing while still breaking
  the config the day the name is retired upstream. The tree is at **zero findings**, so a
  new one is a new fact rather than background noise — keep it that way, and if a rule has
  to go, say why in the file.
- **One pinned linter, installed by the thing that runs it.** `GOLANGCI_VERSION` in the
  [`Makefile`](Makefile) is the only place the version is written; `make lint` depends on
  `lint-deps`, which installs that exact version (upstream's `install.sh`, which is what
  it recommends over `go install`) when the binary is missing or is a different one. So
  local and CI agree by construction — CI installs no linter of its own, it just runs
  `make check`. Bumping the linter is editing one line and reading what goes red. A
  golangci-lint *warning* also fails `make lint`, because the one it emits is
  `warn-unused`: an exclusion rule in `.golangci.yml` that no longer matches anything, so
  dead config cannot sit in the gate reading like a decision.
- `make check` runs in CI on every push and pull request
  ([`.github/workflows/check.yml`](.github/workflows/check.yml)), and installs **no tool of
  its own**: `make lint` and `make dead` each install what they run, so CI and a laptop
  cannot disagree about a version — or, in `deadcode`'s case, about whether the check ran
  at all. It runs `make vendor` first, because `web/vendor/` is gitignored and one test
  asserts the tree matches every pin.
- Comments explain *why*, and name the failure that motivated the code. This repo's
  existing comments are the style guide; match their density and voice. Deleted on sight:
  a line restating the line below it, a banner, a commented-out branch, a `TODO` with no
  owner — `changelog/` holds the future and `git log` holds the past.
- **In `web/ui`, an API comment is JSDoc and nothing else:** a `@typedef` for a payload that
  crosses a layer, `@param`/`@returns` only where the name does not already say it, a
  description adding a fact the signature cannot. [`chat.js`](web/ui/src/lib/chat.js),
  [`answer.js`](web/ui/src/lib/answer.js) and [`diagram.js`](web/ui/src/lib/diagram.js) are
  the reference. Fifteen `jsdoc/*` rules run at `--max-warnings 0`, so a tag naming a
  parameter that no longer exists fails `make check`, not review; in Go the same job is the
  doc comment — identifier first, full stop last (`godot`).
- **The vendored skills are the style source, and *Skills* below is the only routing table.**
  Read the row for what you are touching, `ponytail` before any of them, and nothing else —
  a skill never outranks a rule above.

## Skills

**Seventeen skills are vendored, each with its source recorded in
[`skills-lock.json`](skills-lock.json) — read them, don't re-add them.** `.claude/skills/*` are
symlinks to `.agents/skills/*`, so neither agent's set can drift from the other. The lock
records where each one came from; its `computedHash` is written by the tool that fetched them
and **nothing in this tree can recompute it**, so treat it as provenance, not a seal. This section is rule 23 in full: the routing,
the precedence, and every place a skill is wrong *about this repo*. A skill with no row here
is a skill an agent will apply everywhere.

**Precedence, top wins. A disagreement is not a judgement call:**

1. A **rule** in *Critical rules*, and the invariants under *Architecture*.
2. A **machine-checked config in this tree** — [`.golangci.yml`](.golangci.yml),
   [`web/ui/eslint.config.js`](web/ui/eslint.config.js), the `spec.json` join, the
   [`Makefile`](Makefile) pins.
3. The **vendored skill**, as the bytes in this tree read.
4. Nothing else. Recalled style, an upstream README, a newer release note: not a source here.

When a skill loses, the loss goes in `changelog/`, dated, naming the rule it lost to — that is
what stops the next agent re-deriving it. And on its own authority a skill may never add a
dependency (rule 20), a root document (rule 18), a test file (rule 21), or a deferred marker
(rule 24): those are rules, and the skill is tier 3.

**Read lazily.** `SKILL.md` is the contract; a `references/*.md` is opened only when a line in
that SKILL.md points at the case in front of you. Vendoring is not permission to load
`.agents/skills/` into context — the nine Go skills' references are a longer read than the
packages they describe, and an agent that spends its context on style has none left for the
code.

### Which skill, in which order

| touching | read, in this order | from |
|---|---|---|
| any code at all | `ponytail` — laziest thing that works, YAGNI, stdlib before code: rules 17 and 20 written by somebody else | `dietrichgebert/ponytail` |
| `internal/*`, `cmd/*` | `golang-code-style`, `golang-naming`, `golang-error-handling`, then whichever matches: `golang-context`, `golang-concurrency`, `golang-data-structures`, `golang-design-patterns` | `samber/cc-skills-golang` |
| a doc comment, a README | `golang-documentation` | same |
| [`.golangci.yml`](.golangci.yml) | `golang-lint` — this repo's config started from it | same |
| `web/ui/src/lib/*.js`, `web/ui/src/composables/*.js` | `modern-javascript-patterns` | `wshobson/agents` |
| `web/ui/**/*.vue` | `vue`, then `building-components` | `antfu/skills`, `vercel/components.build` |
| a JSDoc contract on a front-end file — a `@typedef` for a payload that crosses a layer, a composable's typed surface | `vue-expert-js` — Vue 3 in JavaScript, typed by JSDoc instead of TS, which is this repo's decision written by somebody else. Its *JSDoc* half only; see *deltas* | `jeffallan/claude-skills` |
| [`web/ui/src/router.js`](web/ui/src/router.js), a screen behind a route | `vue-router-best-practices` — after that file's own header | `hyf0/vue-skills` |
| `vite.config.js`, the bundle | `vite` | `antfu/skills` |
| `eslint.config.js`, `package.json`, tooling | `antfu` | `antfu/skills` |

`vue`, `modern-javascript-patterns`, `golang-code-style`, `golang-naming`, `golang-context`,
`golang-concurrency` and `golang-data-structures` apply as written — but inside the four-layer
table under *Front end* and rules 9–12. A `<script setup>` holding a branch is a composable
nobody wrote yet, however the skill's example is shaped.

### Where a skill is wrong about this repo

| skill | what it assumes | what this repo does |
|---|---|---|
| `ponytail` | marks a deliberate corner with a `ponytail:` comment naming its ceiling and upgrade path | **Rule 24: no deferred marker.** `godox` runs on its default keywords and `no-warning-comments` lists `todo`/`fixme`/`xxx`/`hack`, so a `ponytail:` note would pass the gate while being exactly the lie rule 24 is aimed at. The ceiling goes in `changelog/`, dated. Every other line of the skill is rules 17 and 20 |
| `golang-error-handling` | `samber/oops` and `slog`, and cross-refs `golang-samber-oops`, `golang-samber-slog`, `golang-observability`, `golang-safety` | `go.mod` has two dependencies and neither is a logger: stdlib `errors`, `%w`, `errors.Is`, and `log.Printf`. Those four cross-referenced skills are **not vendored** — a pointer to one is a dead end, not an errand |
| `golang-documentation` | generates README, CONTRIBUTING, CHANGELOG and `llms.txt` from templates | Rule 18 — a `CONTRIBUTING.md` is the fifth root document; `changelog/` is dated session handoffs, not a release changelog; `llms.txt` is generated by `cmd/rendocs` (rule 16). Its *writing principles* are the half that applies |
| `golang-design-patterns` | `ddd`, `clean-architecture`, `hexagonal-architecture`, DI containers | Rule 20 — `cmd` → `internal` → nothing is the architecture, and the three narrow interfaces in `internal/server` are the whole DI story. The constructor, enum, `//go:embed`, compile-time-check and `resource-management` parts apply; the architecture references need a `changelog/` decision first |
| `golang-lint` | its recommended `.golangci.yml` | This repo's started there, then set `linters.default: none`, so `enable:` is the complete set and a rejected linter is a **comment with its reason**, never a `disable:` key. The file is the truth; the skill is where it came from |
| `vue-router-best-practices` | Vue Router 4 | `web/ui/package.json` pins **vue-router 5.2.0**. Check a claim against the installed version before acting on it — the same trap as the unversioned design-system docs site in [`AGENTS.md`](AGENTS.md) |
| `building-components` | TypeScript types, `as-child`/polymorphism, publishing to npm or a registry | JavaScript, nothing published, and 8bit-nes owns components. `principles`, `accessibility`, `composition` and `state` apply; `types`, `npm`, `registry`, `marketplaces`, `as-child` and `polymorphism` do not, and `design-tokens`/`styling` are the library's job (`AGENTS.md`) |
| `vue-expert-js` | Pinia stores, Vitest suites, and cross-refs into a `vue-expert` skill | Only its JSDoc half applies, and *Conventions* is the version of record. There is no store — state is one composable per concern inside the four-layer table (rule 20), no Vitest and no test file for a change (rule 21), and `vue-expert` is **not vendored**, so its `references/*` are dead ends. It agrees with this repo on the one thing that matters: JavaScript with JSDoc, never TS |
| `vite` | `vite.config.ts`, TypeScript | `web/ui/vite.config.js` — JavaScript by decision (see *Front end*). `package.json` pins Vite 8, so the Rolldown half is the present, not a migration to plan |
| `antfu` | scaffolds a project: pnpm, monorepos, library publishing | One npm app with a committed `package-lock.json`, not published. `references/antfu-eslint-config.md` is the useful half; `eslint.config.js` already exists and every rule in it is annotated |
| the `golang-*` audit modes | `ultracode` and up to five parallel sub-agents per audit | Opt-in only, never the default for a change. `make check-full` is the audit of record, and its *skipped* lines are part of the reading |

### Deliberately not vendored

Three were vendored and are now deleted — 189 KB and 37 reference files for surfaces this
repo does not have. The **line is what stops them coming back**, and it costs no bytes:

- **`pnpm`** (`antfu/skills`) — this repo is npm, with a committed `package-lock.json`.
- **`antfu-design`** (`antfu/skills`) — UnoCSS-first, while layout here is
  `web/ui/src/styles.css` over 8bit-nes.
- **`code-documenter`** (`jeffallan/claude-skills`) — Python docstrings, OpenAPI/Swagger,
  FastAPI/Django/NestJS, doc portals. None of that exists here, and the question is already
  answered: JSDoc as *Conventions* describes it, `godoc` in Go.

Each names its upstream, so re-vendoring one is a fetch rather than an archaeology dig — that
is the part a deletion has to leave behind.

Keeping the files was the older answer, on the theory that a deleted skill is a re-added
skill. It is not: a name with its reason does that job, and 189 KB of advice about the wrong
package manager is 189 KB an agent can read by accident.

Adding or bumping a skill: symlink it under `.claude/skills`, record source in
`skills-lock.json`, and add its row above — plus a row in *deltas* or in *not vendored*.
`TestVendoredSkillsMatchTheirRouting` holds the join (`.agents/skills/*` ↔ `.claude/skills/*`
↔ `skills-lock.json` ↔ these rows), so a skill that is vendored without being routed, or
routed without being vendored, fails `make check`.
