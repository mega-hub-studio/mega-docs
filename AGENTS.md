# Notes for AI agents

Read this before answering questions about this repository or changing its UI. It
exists so you use the authoritative documents instead of recalling details that may
be wrong or out of date.

## Documentation for this project

| | URL |
|---|---|
| **Spec — start here** | <https://mega-hub-studio.github.io/mega-docs/spec.json> |
| Machine index | <https://mega-hub-studio.github.io/mega-docs/llms.txt> |
| Guide — what it is, quick start, how it works | <https://mega-hub-studio.github.io/mega-docs/> |
| BA mode — asking, judging, scoping, answering a gap | <https://mega-hub-studio.github.io/mega-docs/ba.html> |
| Dev — the seams, testing, the knobs | <https://mega-hub-studio.github.io/mega-docs/dev.html> |
| Deploy — hosting for a team | <https://mega-hub-studio.github.io/mega-docs/deploy.html> |
| Full file-by-file reference | [`README.md`](README.md) |

Start at `spec.json`: one entry per documented feature, each naming the section that
defines it, the HTTP routes it is reached through, the environment variables that
configure it, and the Go tests that fail when it breaks. Then `llms.txt` for the prose
index. Both are generated from the published pages by `cmd/rendocs`, so neither can
disagree with them.

**Changing something here is spec-first.** Write the page section, declare its
`data-feature` / `data-api` / `data-env` / `data-test` on the `<section>`, and `make check`
stays red until those names exist in the code. An `/api/` route or a config variable that
no section documents fails the build. The five steps are on the Dev page under
["These pages are the spec"](https://mega-hub-studio.github.io/mega-docs/dev.html#spec).

The app binary does **not** serve these pages, and does not link out to them either — it
is the chat app and nothing else. Do not add doc routes to it.

## Two facts that are easy to get wrong

1. **`knowledge.db` IS the source of truth.** A document is a row — `documents.body` holds
   its text — and the BA WebUI is the only way one enters; nothing writes a corpus file, and
   `CORPUS_DIR` is only the folder `ingest` reads when an operator imports from disk. A
   BA-confirmed answer is a row at `qa/ticket-N.md`, which stays a `.md` path because that is
   what a citation prints, not because a file exists. When asked about backups: **it is
   `scripts/backup.sh`** — a verified `sqlite3 .backup` of `DB_PATH`, nightly by systemd timer
   or `make backup` by hand, landing outside this machine's disk. Do not design a second one,
   and do not reintroduce a corpus directory. The other net is that `Remove` is soft — the
   chunks go, the row keeps its text with a `deleted_at`.
2. **Reads are open; the three writes are gated.** `BA_PASS` guards confirming an
   answer, dismissing a ticket, and importing a document (`POST /api/documents`). An
   unset `BA_PASS` means the instance has *no* write surface — not open writes. Asking,
   reading the queue and filing a gap never need a password.

## Documentation for the design system

The UI is built on **8bit-nes**, and the version this repo pins is:

    8bit-nes@0.16.0

Read the docs **for that version**, not the latest:

| | URL | Why |
|---|---|---|
| Pinned machine index | <https://cdn.jsdelivr.net/npm/8bit-nes@0.16.0/llms.txt> | ships in the package, so it matches the pinned bytes exactly |
| Pinned full reference | <https://cdn.jsdelivr.net/npm/8bit-nes@0.16.0/llms-full.txt> | same |
| Pinned component data | <https://cdn.jsdelivr.net/npm/8bit-nes@0.16.0/components.json> | same |
| Human docs site | <https://tutranmvp.github.io/8bit-components/docs.html> | **always latest** — will describe components this repo does not have |

That distinction matters. The docs *site* is unversioned, so reading it while this
repo pins an older release is how you end up using a class or element that is not in
the CSS actually being loaded. The package's own `llms.txt` is version-exact.

The pin lives in one place, [`web/vendor.sha384`](web/vendor.sha384) — versions *and*
`sha384` digests, for both the page's `integrity` attributes and `make vendor`. A test
(`TestAgentNotesPinMatchesTheManifest`) fails if the version quoted above drifts from
it, so trust the table.

## Three things to know before changing the UI

1. **Measure at 390 first — the phone is the design, not a breakpoint.** Rule 28 in
   [`CLAUDE.md`](CLAUDE.md). Base styles are the phone; one `min-width` query upgrades to
   desktop, and a `max-width` query going down is the wrong direction. Decide the layout at
   390, then widen only if it survives.

   One measurement has a right answer and the rest are judgement: **the page may never scroll
   sideways.** `documentElement.scrollWidth === innerWidth`, at every width. A block too wide
   for a phone scrolls or pans *inside itself* — a `<pre>`, a `.table-wrap` and `<nes-zoom>`
   all do — or it is capped.

   The trap to know, because it looks like the opposite of a bug: 8bit-nes opts a list of
   elements out of `--prose-measure` with `max-inline-size: none`, on its own stated principle
   *"if the content is the width, it opts out"*. That is right for every entry that scrolls
   itself and **wrong for `img`**, which has no such escape — a 1400px screenshot took
   `none` and rendered 1404px inside a 1207px card. A *reading* limit and a *container* limit
   are two caps: an image may escape the measure, nothing may escape the container.

   PinchTab cannot emulate `pointer: coarse` — `set media pointer coarse` reports "applied"
   while `matchMedia` stays false, so **touch-target heights are unchecked here** and rest on
   8bit-nes' own release testing. `scripts/check-docs-ui.mjs` says so at the site.
2. **Do not reimplement a recipe 8bit-nes already ships.** Check its `llms.txt`
   first. `web/ui/src/styles.css` and the `<style>` block in `web/docsbase.html` own
   *layout*; the design system owns components.

   The sharpest edge of that split, and rule 27 in [`CLAUDE.md`](CLAUDE.md): **a recipe
   spaces its own parts and nothing else.** `.field` spaces its label, control and hint;
   `.card > .head`, `.sources` and `.feedback` each carry one margin; `.callout` carries
   none — and `.card` is not a flex container. So the space *between* two recipes is
   always this repo's, it belongs to the container as one `gap`, and a per-child margin is
   how five pairs of blocks ended up touching at 0px on a phone. Add a container, declare
   its `gap`; never a margin on the child, and never nothing.
3. **Every local override of the design system is named, and there are two kinds.** All of
   them are in `web/ui/src/styles.css`, against **8bit-nes 0.16.0**, and each was re-verified
   against that release's own CSS rather than its changelog, because "fixed upstream" is a
   claim about bytes.

   **The first kind is empty**, and has now been emptied twice. Everything this app was
   patching around landed upstream, and each local rule was deleted rather than parked beside
   its replacement — the sites keep one line saying what they fixed, so the loop reads as
   closed. On the 0.16.0 bump only `all.min.css` changed digest; `elements.min.js` and all
   three fonts came back byte-identical, which is what a CSS-only release should look like.

   | landed in 0.16.0 | what the release does instead |
   |---|---|
   | `.prose > img` `max-inline-size: 100%` | `&>:is(img, svg, video) { max-inline-size: 100%; block-size: auto }` — its own rule, not a later override, because `:is()` takes its most specific argument's weight and `.table-wrap` made the opt-out list (0,2,0) |
   | `.prose .table :is(th, td)` `vertical-align: top` | `th` and `td` both ship `top`; the release comment measures the same failure this app did — first-line tops spread 57.8px across one row |
   | the emoji prepended in `lib/answer.js`'s `dressAlerts` | `.callout::before { content: var(--mark) / "" }`, one ASCII character per kind in a reserved gutter, hidden from a screen reader. Better than what was here: a gutter holds for a panel opening with a list, where a character in the first paragraph has no line to join |
   | *(nothing was patched)* `.pbar { --fill }` | `@property --fill` is `inherits: true` now, so setting it on the container works. This app sets it on the `<i>`, which is what the docs always showed, and stays there |

   The 0.15.0 round is not repeated here; it is in `changelog/upstream-8bit-nes-0.14.0.md`
   with what each fix replaced. The counter-example worth keeping is from that round: the
   `display: block` half of a `nes-walkthrough` override had landed in **0.14.0** and sat
   here unnoticed for a release, because nobody re-measured. Re-measure on every bump.

   **This app using a recipe outside the context it was written for.** Nothing upstream to
   fix; each is permanent until the app's own use changes:

   | adjustment | the mismatch |
   |---|---|
   | `.source .source-title:hover` un-underlined | the recipe expects an `<a>`; the row is spans, because nothing in the ASK screen can open a document, so the hover underline offered a link that does not exist |
   | `.lib-row > .result` — `cursor: default`, no hover edge, `padding-inline: 0` | `.result` is written as a control; in the BA library it is a record, and its inset would miss the card edge the panel's head, Find field and buttons all share |
   | `.dock .statusline` — `inline-size: auto`, and `.sl-end` un-pushed below 640px | `inline-size: 100%` fights the dock's negative margins (12px short on the right, measured), and `margin-inline-start: auto` gives a *wrapped* strip two different left edges |
   | `.empty .palette > .palette-empty` — start-aligned, at the rows' inset | centring is right for a state filling a blank list, wrong for a truncation notice under nine left-aligned rows |
   | `::selection` softened to a 32% `--primary` tint | the solid fill is ~11:1 on this dark page — correct contrast, and a flare when a long-press selects a word on a phone |

   All five were re-measured against 0.16.0 and all five are still needed: the recipe still
   underlines `.source-title` on hover, `.result` is still a control (`cursor: pointer`, a hover
   edge, `--pad-snug`), `.statusline` is still `inline-size: 100%` with `.sl-end` pushed,
   `.palette-empty` is still centred, and `::selection` is still a solid fill. One of those
   nearly went the other way on a careless grep — a three-line window around `.source-title`
   missed its nested `&:hover` and read as "landed". Widen the window before deleting a rule;
   a wrongly deleted override is a defect that ships looking like housekeeping.

   `.explain` is a sixth, and the only one that is an *addition* rather than a correction: it
   is this app's own callout kind, so 0.16.0 has no `--mark` for it and `styles.css` supplies
   one (`~`) beside its `--teal`. A kind the library does not know about has to bring both.

   The requests behind each round, and the prompts they were sent as, are in
   `changelog/upstream-8bit-nes-0.14.0.md` and `changelog/upstream-8bit-nes-0.15.0.md`.

   `.prose` used to be a fourth. Its `72ch` sat on the container and so capped the tables and
   diagrams inside an answer as well as its text; 0.8.0 moved that measure onto the children
   (`--prose-measure`) with the width-is-content constructs opting out, which is what this app
   had been patching around, so the override was deleted rather than re-pinned. That is the
   shape to aim for: report it, and delete the local rule when it lands.

   `docsbase.html` overrides nothing — it used to patch two accessibility gaps, and 0.7.1
   ships both, so they were removed after measuring rather than assumed.

   The other app rules on library classes are *placement* or a state the library has no
   recipe for, not overrides, and the difference is worth keeping straight:
   `align-self`/`text-align` on `.empty`'s children decide where a child sits inside its
   parent (the app's job), `.source:target` gives a citation jump the one thing it lacked
   — a visible reply — the way `.drop`'s dashed edge does for a drop target, and the `.lib-*`
   rules decide where a `.result` row sits and how wide its two number columns are;
   `max-block-size` on `.palette-list` overrules how the component sizes itself (the
   library's job).

   Four classes in a template are worth knowing about, because a class that matches nothing
   fails **silently**: `.row` and `.grow` do not exist on their own (the library has
   `.tree .row` and `.control-group.row`; `.grow` is this app's `.bar .grow`), `.hint` exists
   only as `.field > .hint`, and `.perm` is a confirmation *block*, not a button modifier. The
   BA library's list carried all four at once, which is what turned it into two ragged columns
   of shattered paths. There is no linter for this — `grep` the class in the built CSS.

   Adding an override is allowed; leaving it anonymous is not. Say in a comment which
   upstream version lacks the fix, and if you bump the pin, re-measure rather than trusting
   the changelog. A release has already been tagged twice without a fix it was expected to
   contain.

## Verifying, not guessing

- `make check` is the gate: tests, `gofmt`, `go vet`, `staticcheck`, `deadcode`, **ESLint
  over `web/ui`** (it is the formatter too — style rules are errors), and a scan that
  refuses credential-shaped strings in tracked files.
- `internal/aitest` is a fake OpenAI-compatible provider, so the whole pipeline is
  testable with **no API key and no network**.
- Never put a real key in a file, a command line, or a commit. Keys come from `.env`,
  which is gitignored.
- A commit message follows [`.vscode/commit.instruction.md`](.vscode/commit.instruction.md) —
  the only place this repo's version lives.
- **A user-visible change is not finished until it is in the release modal** (rule 29 in
  [`CLAUDE.md`](CLAUDE.md)). The version badge in the app opens a list of what changed, and
  that list is **generated from `git log`** by `make release V=vX.Y.Z` — `web/release.json`
  carries a do-not-edit marker and `TestReleaseNotesAreGenerated` fails on a hand-written one.
  So there is nothing to type into the modal; there are two things to get right:

  1. **The commit subject is the line the user reads.** It is rendered verbatim, under its
     scope, beside its sha. Write it for someone looking at the app, not for the diff.
  2. **Cut the release** on a clean tree that is `make check-full` green. Skip it and the badge
     keeps naming the previous version while the binary is something else — the modal then
     describes work the running code no longer matches, which is the one screen a user has for
     answering "what changed?".
