# 2026-07-27 — Four pages, one per role, split by feature

The docs had grown into three long pages where a reader's own question was somewhere in
the middle of somebody else's. Asked to restructure them: fewer detours, a Pareto quick
start, a page per mode, sub-modules per feature, and every flow drawn rather than
described.

## The shape now

| page | who arrives here | sections |
|---|---|---|
| `index.html` (Guide) | everyone, once | Start in 60 seconds · How it works · When the answer isn't there · What the app can do · Getting in, the first time · When something's wrong |
| `ba.html` (**new**) | BA / PM / support, daily | 1 · Ask so retrieval finds it · 2 · Trust the answer? · 3 · Narrow to one folder · 4 · File a gap and answer it · 5 · Import documents · 6 · What an answer cost · 7 · When something looks wrong |
| `dev.html` | DEV, first hour | + How an answer is built · + The front end, in four layers |
| `deploy.html` | whoever hosts it | + First install |

Two rules drove the split, and both are checkable rather than tasteful:

- **Nothing appears twice.** The four habits, the folder picker, the importer and the cost
  strip moved off the Guide to the BA page; the Guide keeps a pointer, because the reader
  who got that far is the reader who needs it. `dev.html` and `ba.html` both cover the QA
  loop and are *not* duplicates — one is filing a gap, the other is answering one — so
  each now links the other's half instead of restating it.
- **The first thing on the Guide is a first answer.** "Start in 60 seconds" is three steps
  and no terminal, with a 20/80 callout pointing at the three things that pay for
  themselves immediately (scope, import, cost). The three host commands that used to open
  the Guide moved to `deploy.html#install`, where the person who runs them is.

Sections are numbered on the BA page only, because there it *is* a sequence — the order
the four moves come up in a day. Elsewhere numbering would be decoration.

## Two new diagrams

`web/retrieval.mmd` → the Dev page: question + scope → cache? → embed → vectors ·
keywords → RRF fuse → top K → prompt → stream → store. `web/loop.mmd` → the BA page: ask
→ miss → Ask BA → queue → draft → confirm → file in `qa/` → indexed → cited → free.

Both are rendered by `make diagram` and **committed as SVG**; mermaid never ships to a
docs page. Same two constraints as the first diagram, both from the phone: shallow, and
short lowercase-distinct labels.

## A real bug, found by measuring

On a phone `<nes-toc>` is a collapsed bar whose whole job is to name the section you are
in. After one tap on VI/EN it went blank — and stayed blank, on every page, forever.

The cause is in the component: setting an observed attribute rebuilds the index, and the
scroll-spy re-seeds itself with the first heading, but its `_mark()` skips an id it
believes is already active. A language switch writes **two** attributes (`label` and
`levels`), so the second rebuild always matched and the fresh span was never filled.

Fixed in both places, for two different reasons:

- `8bit-nes` (`elements.js`): the rebuild now forgets the active id first — the cached one
  described rows that no longer exist. Committed as **0.7.3** — version bumped, bundle and
  `sri.json` rebuilt, README/example pins moved — but **not published**: `pnpm publish` is
  a human's call. Until it is on npm this repo stays pinned to **0.7.2**, so the fix
  reaches these pages only when `web/vendor.sha384` is bumped after a release.
- `web/docsbase.html`: `label` is written *before* `levels`, so the last rebuild is the one
  that also changes which headings are indexed — the one case the old component handles.
  Load-bearing order, with the reason written next to it.

Verified on the published order of writes with both bundles: the old bundle + old order is
blank on all four pages; either fix alone repairs all four.

## What the check covers now

`web/embed_test.go` and the Pages workflow both count four pages: every page exists, is
non-empty, has no surviving `<%` action, links the other three, and appears in `llms.txt`.
The llms.txt section names were updated with the pages — that test asserts one heading per
page, so a renamed section fails it.

Browser-measured, on a phone (390) and a laptop (1440), both languages, all four pages:
no page scrolls sideways, no `<nes-toc>` row wraps, every toc row points at an id that
exists, no section is missing its heading in the reading language, wide content scrolls
inside its own box, every cross-page link and fragment resolves, every page reaches the
other three, the phone's chrome stays at 44px, and all three diagrams paint inside their
container.
