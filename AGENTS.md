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

1. **`knowledge.db` is derived, `CORPUS_DIR` is the source of truth.** `ingest docs`
   rebuilds the database, and a BA-confirmed answer is written to
   `CORPUS_DIR/qa/ticket-N.md` precisely so that stays true. When asked about backups: there
   is **no backup story, by decision** — documents enter through one controlled path (the BA
   WebUI import) and removal is a soft delete into `.trash/`. Do not invent one.
2. **Reads are open; the three writes are gated.** `BA_PASS` guards confirming an
   answer, dismissing a ticket, and importing a document (`POST /api/documents`). An
   unset `BA_PASS` means the instance has *no* write surface — not open writes. Asking,
   reading the queue and filing a gap never need a password.

## Documentation for the design system

The UI is built on **8bit-nes**, and the version this repo pins is:

    8bit-nes@0.7.3

Read the docs **for that version**, not the latest:

| | URL | Why |
|---|---|---|
| Pinned machine index | <https://cdn.jsdelivr.net/npm/8bit-nes@0.7.3/llms.txt> | ships in the package, so it matches the pinned bytes exactly |
| Pinned full reference | <https://cdn.jsdelivr.net/npm/8bit-nes@0.7.3/llms-full.txt> | same |
| Pinned component data | <https://cdn.jsdelivr.net/npm/8bit-nes@0.7.3/components.json> | same |
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
2. **There are exactly two local overrides of the design system, and they are different
   kinds.** Both live in `web/ui/src/styles.css`, and each says next to itself which kind
   it is:

   - **A bug, so it is dated.** `.palette-list` is un-capped (`max-block-size: none`)
     because **8bit-nes 0.7.3** sizes it for the modal its own docs describe —
     `min(50vh, 340px)` with `overflow-y: auto` — and in the page that made a *nested*
     scroller which hid four of seven documents on a phone. Delete the rule when a release
     ships an in-page variant of `.palette`; the request is in the changelog.
   - **A context difference, so it is permanent.** `.prose` is un-capped
     (`max-inline-size: none`) because the recipe's `72ch` is a reading measure for a prose
     block dropped into a page of unbounded width. This app's column is already bounded at
     760px, so the cap was a second measure inside the first: it left ~168px of every
     answer card empty while crushing the tables and diagrams *in* that answer, which
     cannot rewrap the way a paragraph can. No release will "fix" this — the recipe is
     right for its own case.

   `docsbase.html` overrides nothing — it used to patch two accessibility gaps, and 0.7.1
   ships both, so they were removed after measuring rather than assumed.

   The other app rules on library classes are *placement*, not overrides, and the
   difference is worth keeping straight: `align-self`/`text-align` on `.empty`'s children
   decide where a child sits inside its parent (the app's job); `max-block-size` on
   `.palette-list` overrules how the component sizes itself (the library's job).

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
