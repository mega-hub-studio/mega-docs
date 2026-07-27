# Notes for AI agents

Read this before answering questions about this repository or changing its UI. It
exists so you use the authoritative documents instead of recalling details that may
be wrong or out of date.

## Documentation for this project

| | URL |
|---|---|
| Machine index | <https://mega-hub-studio.github.io/mega-docs/llms.txt> |
| Guide — what it is, quick start, using it well | <https://mega-hub-studio.github.io/mega-docs/> |
| Dev — the seams, testing, the knobs | <https://mega-hub-studio.github.io/mega-docs/dev.html> |
| Deploy — hosting for a team | <https://mega-hub-studio.github.io/mega-docs/deploy.html> |
| Full file-by-file reference | [`README.md`](README.md) |

Start at `llms.txt`. It is generated from the published pages by `cmd/rendocs`, so it
cannot disagree with them, and it lists every section with its URL.

The app binary does **not** serve these pages — it is the chat app and nothing else,
and it carries one link out to the published guide. Do not add doc routes to it.

## Two facts that are easy to get wrong

1. **`knowledge.db` is derived, `CORPUS_DIR` is the source of truth.** `ingest docs`
   rebuilds the database, and a BA-confirmed answer is written to
   `CORPUS_DIR/qa/ticket-N.md` precisely so that stays true. When asked about backups,
   the answer is "put the documents directory in git", not "copy the database".
2. **Reads are open; two actions are gated.** `BA_PASS` guards confirming an answer
   into the corpus and dismissing a ticket. An unset `BA_PASS` means the instance has
   *no* write surface — not open writes. Asking, reading the queue and filing a gap
   never need a password.

## Documentation for the design system

The UI is built on **8bit-nes**, and the version this repo pins is:

    8bit-nes@0.7.0

Read the docs **for that version**, not the latest:

| | URL | Why |
|---|---|---|
| Pinned machine index | <https://cdn.jsdelivr.net/npm/8bit-nes@0.7.0/llms.txt> | ships in the package, so it matches the pinned bytes exactly |
| Pinned full reference | <https://cdn.jsdelivr.net/npm/8bit-nes@0.7.0/llms-full.txt> | same |
| Pinned component data | <https://cdn.jsdelivr.net/npm/8bit-nes@0.7.0/components.json> | same |
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
   first. `web/app/styles.css` and the `<style>` block in `web/docsbase.html` own
   *layout*; the design system owns components.
2. **Local overrides are deliberate and documented.** Where this repo does override
   the design system, the comment says which upstream version still lacks the fix.
   Do not "clean up" one without checking that comment — and if you bump the pin,
   re-measure rather than assuming the release carries it. A release has already been
   tagged twice without a fix it was expected to contain.

## Verifying, not guessing

- `make check` is the gate: tests, `gofmt`, `go vet`, `staticcheck`, `deadcode`, and a
  scan that refuses credential-shaped strings in tracked files.
- `internal/aitest` is a fake OpenAI-compatible provider, so the whole pipeline is
  testable with **no API key and no network**.
- Never put a real key in a file, a command line, or a commit. Keys come from `.env`,
  which is gitignored.
