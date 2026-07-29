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

    8bit-nes@0.13.0

Read the docs **for that version**, not the latest:

| | URL | Why |
|---|---|---|
| Pinned machine index | <https://cdn.jsdelivr.net/npm/8bit-nes@0.13.0/llms.txt> | ships in the package, so it matches the pinned bytes exactly |
| Pinned full reference | <https://cdn.jsdelivr.net/npm/8bit-nes@0.13.0/llms-full.txt> | same |
| Pinned component data | <https://cdn.jsdelivr.net/npm/8bit-nes@0.13.0/components.json> | same |
| Human docs site | <https://tutranmvp.github.io/8bit-components/docs.html> | **always latest** — will describe components this repo does not have |

That distinction matters. The docs *site* is unversioned, so reading it while this
repo pins an older release is how you end up using a class or element that is not in
the CSS actually being loaded. The package's own `llms.txt` is version-exact.

The pin lives in one place, [`web/vendor.sha384`](web/vendor.sha384) — versions *and*
`sha384` digests, for both the page's `integrity` attributes and `make vendor`. A test
(`TestAgentNotesPinMatchesTheManifest`) fails if the version quoted above drifts from
it, so trust the table.

## Two things to know before changing the UI

1. **Do not reimplement a recipe 8bit-nes already ships.** Check its `llms.txt`
   first. `web/ui/src/styles.css` and the `<style>` block in `web/docsbase.html` own
   *layout*; the design system owns components.
2. **Every local override of the design system is named, and there are two kinds.** All of
   them are in `web/ui/src/styles.css`, against **8bit-nes 0.13.0**, and each was re-verified
   against that release's own CSS rather than its changelog, because "fixed upstream" is a
   claim about bytes.

   **Waiting on a release. Re-measure on the next bump, and delete what has landed:**

   | override | why |
   |---|---|
   | `.palette-list` un-capped (`max-block-size: none`) | the library still sizes it for the modal its own docs describe — `min(50vh, 340px)` with `overflow-y: auto` — and in the page that made a *nested* scroller which hid four of seven documents on a phone. Gone when a release ships an in-page `.palette` |
   | `.prose a.cite` restored to cyan with no underline | two of the library's own recipes collide: `.prose a` and `.cite` are both in the components layer and `.prose a` scores higher, so every citation marker rendered as a prose link — green, with a 2px underline through a digit already sitting in a cyan chip. Gone when a release scopes that rule away from `.cite` |

   **This app using a recipe outside the context it was written for.** Nothing upstream to
   fix; each is permanent until the app's own use changes:

   | adjustment | the mismatch |
   |---|---|
   | `.source .source-title:hover` un-underlined | the recipe expects an `<a>`; the row is spans, because nothing in the ASK screen can open a document, so the hover underline offered a link that does not exist |
   | `.lib-row > .result` — `cursor: default`, no hover edge, `padding-inline: 0` | `.result` is written as a control; in the BA library it is a record, and its inset would miss the card edge the panel's head, Find field and buttons all share |
   | `.dock .statusline` — `inline-size: auto`, and `.sl-end` un-pushed below 640px | `inline-size: 100%` fights the dock's negative margins (12px short on the right, measured), and `margin-inline-start: auto` gives a *wrapped* strip two different left edges |
   | `.empty .palette > .palette-empty` — start-aligned, at the rows' inset | centring is right for a state filling a blank list, wrong for a truncation notice under nine left-aligned rows |
   | `::selection` softened to a 32% `--primary` tint | the solid fill is ~11:1 on this dark page — correct contrast, and a flare when a long-press selects a word on a phone |

   The two upstream requests are in the changelog.

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
