# 2026-07-28 — The first screen is a menu of documents, not three sentences

Asked for: make this area read as a menu, evenly aligned, and lean on the design system
rather than app CSS. Three separate failures came out of it, and only the first was the one
that was visible in the screenshot.

## 1. `.suggest` was the wrong recipe, the same way `.steps` was

The three starters were `.suggest` + `.suggest-item`, which is the library's recipe for
**short follow-up chips**: `flex-wrap: wrap`, with each pill sized to its own text. Given
sentence-length labels — "What does supporting calendar controls dev handoff cover?" — it
sized three pills differently and wrapped them one-then-two. Nothing was misaligned by
accident; the recipe was doing exactly what it is for, on content it is not for.

Same class of mistake as `.steps` (a stage bar) used for numbered instructions, which is
already a trap in CLAUDE.md. The lesson generalises: in this design system a recipe's *name*
describes its content, not its shape.

`.palette` + `.result` is the recipe whose content this is — a search box over a result
list, each row an icon, a title, a dim path and a trailing hint. Measured after, on an
iPhone 14 and at 1440: **seven rows, one width, one height, one 2px row gap, one left edge,
one icon column, one right edge for the counts, zero wrapped titles.** Long names ellipse
instead of wrapping — the deliberately absurd 77-character fixture title included.

## 2. The three sentences were also hiding four documents

`starters()` took the top three documents by section count and built a sentence for each.
With seven indexed, four had no row and no way to be reached from the first screen.

The menu is the corpus now, ranked by retrievable sections — which is what makes a document
*able* to answer, and the count is on every row so the order never has to be taken on
trust — with a field that narrows it by title **and path**, because a folder name is how
anyone with a structured corpus actually looks and it is invisible in the title.

The row shows the document; the tap still sends "What does <title> cover?". Since the row no
longer displays that sentence, a test drives the whole path: tapping row one sends the full
question (not the ellipsed label), and the answer streams with its citation.

## 3. Two defects the measurements could not see

Numbers said the layout was perfect while the screenshot said otherwise, twice. Both are
worth writing down because both were *library* semantics I had assumed.

- **`.badge.clear` is not "quiet", it is `--accent: var(--good)`** — the green "passed"
  status fill. On a section count it claims a pass state that means nothing, and it rendered
  as the loudest thing on the row. The pre-existing code used it for the scope and hit
  counts too. Plain `.badge` is the neutral one (`--muted` on `--panel-2`); the per-row count
  is now the recipe's own unstyled `.result-hint` slot, in mono tabular figures so a column
  of numbers lines up.
- **`.palette-list` caps itself at `min(50vh, 340px)` with `overflow-y: auto`.** Right for a
  palette floating in a modal; in the page it made a *nested* scroller, and on a phone it
  showed three of seven documents with the other four behind an inner scrollbar. That is the
  bug from §2 again, in a third disguise, and a scroll trap on touch. The app un-caps the
  height so the page is the only scroller, and caps the **row count** instead
  (`MAX_ROWS = 8`, roughly the same visual budget). The header reads `shown / total` and a
  line appears when there are more, so a corpus bigger than the menu is never truncated in
  silence.

## What the phone actually shows

Worth stating rather than implying pixel perfection: on a 390×664 viewport the header, the
title, the metric bar, the filter and **three rows** are above the fold; the rest need a
scroll. That is inherent to a 172px fixed dock plus `.empty`'s icon-and-title block, not
something the row cap can fix. Hit-testing every row confirms what matters — at the bottom
of the page all seven rows hit themselves, so nothing is permanently behind the dock.

The metric bar replaced a paragraph for a reason: inside `.empty`'s `max-inline-size: 42ch`
the three facts wrapped mid-phrase ("Every claim / cited."). As `.note-stats` each fact is
one atomic `<span>`, so the phone break falls *between* facts — measured at 2 lines,
centred to the pixel (53/53 and 108/108).

## Layering

The branch is a pure function in `lib/library.js` (`rankDocs`), the reactive state is one
`ref` in the new `composables/finder.js`, and the component composes it — so
`TestComponentsHoldNoLogic` and `TestComposablesDoNotImportEachOther` both stay true without
being worked around. `lib/library.js` lost `starters()` and gained `rankDocs`,
`coverQuestion` and an exported `docTitle`; `useCorpus` lost its `starters` computed and
`App.vue` one prop.

Two dead selectors went with them: `.corpus .sources` and `.corpus .source` matched nothing
(the only `.sources` list lives in `ChatTurn`, outside any `.corpus`), and `.corpus`'s
`margin-block-start` was making two of the panel's six seams wider than the other four —
`.empty` already spaces every child with one `gap`.

`make check` green, ESLint 0, golangci-lint 2.12.2 at 0, no console or network errors in any
run.

## Still open

Unchanged from this morning: the host has not been redeployed, and `/opt/knowledge/corpus`
still has no remote.

New, and now clearly worth doing: **the fake provider used to verify all of this is still
not committed.** Every claim above was measured against the real binary serving a
seven-document fixture, ingested through a 60-line OpenAI-compatible stub (deterministic
embeddings at the schema's 1536 dimensions, a streaming chat reply) — with no key and no
cost. It has been rewritten from scratch three times now in three sessions, which is the
signal it should be a file. Acceptance: a documented target that ingests a fixture and
serves the UI with `AI_API_KEY` unset.
