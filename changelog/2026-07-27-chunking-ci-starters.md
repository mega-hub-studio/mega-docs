# 2026-07-27 — Chunking measured and fixed, CI, corpus-derived starters

Four items from a prioritised sweep, each one chosen because a measurement said so
rather than because it sounded like an improvement. One item was investigated and
deliberately **not** built.

---

## 1. Chunking: half the index was too small to retrieve, and ten tables too big

Measured on the five specification documents (before they left this repo):

| | before | after |
|---|---|---|
| chunks | 471 | 296 |
| median chunk | 315 chars | **789** |
| under 300 chars | **228** | 21 |
| over `maxChars` (2400) | **10** | 0 |
| largest | **11,651** | 2,385 |
| context per answer (`TOP_K=6` × median) | ~1,890 chars | **~4,730** |

Two root causes, both in `internal/rag/chunk.go`:

**Undersized sections were never merged.** The chunker split oversized sections and
left everything else alone, and a specification is mostly short numbered
sub-headings — so 228 of 471 chunks were under 300 characters, the smallest 19. Each
one competed for one of six `TOP_K` slots it could not fill. Consecutive small
sections now merge until they pass `minChars` (600), and stop there: filling all the
way to `maxChars` would trade a small section's precision for a big one's recall, and
the point is only to get off the floor.

A merged chunk keeps both halves of its provenance — the breadcrumb becomes the
deepest heading the merged sections *share* (`sharedCrumb`), and each section's own
heading stays inline in the text. Without the inline copy the first section is the one
that loses its name; without the shared breadcrumb the citation misattributes the
others.

**A markdown table has no blank lines, so paragraph packing could not split it.** All
ten oversized chunks were business-rules tables, the largest 11,651 characters — one
of those is most of an answer's context, and the model has to find one rule inside it.
`splitLines` now breaks an oversized paragraph on line boundaries, and **repeats the
header row and separator in every part**: without them, part 2 onwards is a grid of
values whose columns have no names.

Tests: `internal/rag/chunk_test.go` — the merge, the standalone case, the
`maxChars` guarantee, the table split, and `sharedCrumb`.

**This needs a re-ingest to take effect.** Existing rows keep the old cut:

```bash
sudo systemctl stop knowledge
rm -f state/knowledge.db state/knowledge.db-wal state/knowledge.db-shm
./bin/ingest corpus
sudo systemctl start knowledge
```

## 2. CI: `make check` on every push and pull request

`.github/workflows/check.yml`. Until now the gate ran on a laptop and on the deploy
host — so a broken commit reached `main` looking fine and failed *during* a
deployment, after the pull, with the service already stopped. It runs `make vendor`
first (the vendored tree is gitignored and one test asserts it matches every pin), and
installs staticcheck and deadcode so the Makefile skips nothing. Both run clean on
this tree, verified locally before the workflow was written.

## 3. Starters now come from the corpus

The empty screen asked three hardcoded questions about this engine — over a corpus of
booking specifications, a first screen advertising the wrong subject, where all three
return "not in the documents". `library.starters()` now names the three biggest
indexed documents: *"What does <title> cover?"* — the question that proves retrieval,
grounding and citation in one tap. Verified: tapping the first one cites §1. Purpose
of that document.

## 4. The schema has no migrations — written down rather than fixed

`schema.sql` is `CREATE TABLE IF NOT EXISTS`, applied on every start, so **a new
column never reaches a database that already exists**: queries naming it pass locally
against a fresh file and fail at runtime on the deployed instance.

No migration runner was added, because the database is derived (invariant 1) and the
upgrade path is to rebuild. What was missing was anyone knowing that, so it is now in
`CLAUDE.md` with the commands, and on the Deploy page as the three kinds of change
that need a rebuild: a chunking change, a new column, a change to how a path is
stored. Everything else — a prompt edit, a new model, a retrieval parameter —
invalidates the answer cache by itself and needs nothing.

## Investigated and not built: a stopword filter for BM25

The hypothesis was that `toFTSQuery`'s OR-of-every-term would let corpus-common words
dominate — plausible, because they really are common: over this corpus "Booking"
appears in 88% of chunks, "calendar" 70%, "list" 51%.

It does not happen. FTS5 ranks with BM25, which weights by inverse document frequency,
so a term in 88% of chunks contributes almost nothing. Asked
*"BR-BL-01 quy định gì về Booking List popup?"* with every term OR'd, the top four
results are all in the one document that defines `BR-BL-*` codes — the rare term
decides the ranking, exactly as it should.

So: no filter. It would be code with no defect behind it, and a hand-rolled stopword
list over a corpus that changes weekly is a maintenance cost that buys nothing.
Recorded here so the next person does not re-hypothesise it.

## Still open

- **The corpus repo still has no remote**, and must be private.
- **Re-ingest re-embeds everything** — no content hash, so an automated ingest would
  pay for unchanged files. Deferred deliberately: it needs a column, therefore a
  rebuild (§4), and the corpus is small enough that a full re-ingest is cheaper than
  the machinery. Trigger to build it: a corpus in the hundreds of files, or ingest
  moving off a human's hands.
