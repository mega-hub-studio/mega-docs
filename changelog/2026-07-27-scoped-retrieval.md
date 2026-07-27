# 2026-07-27 — Scoped retrieval: ask inside one folder

The open item from the deployment entry (§3b, "Tree UI + scoped retrieval") is done.
It was deliberately not started while the corpus held three documents about this
project; five real handoff documents landed in `docs/booking/**`, three folders deep,
which is the corpus a scope filter can actually be judged against.

Those documents were removed from this repository shortly afterwards, and `docs/*` is
now gitignored — correctly: this repo is public, and a corpus belongs in a private one.
Everything measured below was measured against them while they were in the tree (471
sections), which is why the numbers name a corpus you will not find here.

---

## What it does

Tap a folder in **Browse documents** and the next answer is built from that subtree
only — retrieved from it, cited from it, and cached under it. The chosen folder stays
visible above the prompt, every answer keeps a badge naming it, and **ALL DOCS**
widens back out. `POST /api/chat` takes `scope`; `""` is the whole corpus, which is
what every question was before this.

No new endpoint. `GET /api/corpus` already returned full paths, so the tree is derived
client-side in `web/app/tree.js` from data the app was already fetching.

## The two things that could have broken it silently

Both fail as a *confident answer*, never as an error, so both have a test.

### 1. Filtering after fusion would have thinned every scoped answer

The scope is a pre-filter on **both** retrievers — the vector KNN via
`chunk_id IN (SELECT …)`, BM25 via the same subquery on `rowid` — so `TOP_K` counts
matches inside the folder. Dropping out-of-scope rows after RRF would return fewer
than `TOP_K` sections and quietly degrade the answer.

That depends on `sqlite-vec` honouring a rowid constraint as a *pre*-filter rather
than post-filtering a global top-k. It does, as of v0.1.6 — measured, not assumed:
`TestScopedSearchRanksWithinTheScope` stacks twenty nearer out-of-scope chunks against
five far in-scope ones and asserts `k` in-scope hits come back. A version that
regressed that would fail the test instead of shipping thin answers.

`d.path = ? OR d.path LIKE ? ESCAPE '\'`, and the `LIKE` argument is escaped: scoping
to `q_1` must not also answer from `qa1`, and `book` must not match `booking`.

### 2. The cache would have answered one folder from another

The key is now `scope + question`, so the same words asked in two folders are two
questions. The scope is deliberately **not** in `corpus_sig`: a signature is *pruned*
when it changes, so scoping it would empty every other folder's cache on each pick —
and the History panel exists precisely to say "this one is free".

No schema change. The scope lives inside `q_norm` (separator `\x1f`), which means
rows cached before scopes existed still serve unscoped questions, and the deployed
database needs no migration — there is no migration mechanism, only
`CREATE TABLE IF NOT EXISTS`. `History` reads the scope back out of the key, so a
replay restores the scope it was answered in rather than buying a completion from a
panel labelled "free to repeat".

`rag.Scope` canonicalises the string in one place (`path.Clean` against a leading
slash): `booking/`, `/booking` and `booking/./` are one scope, not three cache
entries where two are paid for twice.

## Verified

`make check` plus a real browser against the real corpus (471 sections, fake provider
for embeddings and chat):

- scoping to `booking/calendar` cited only that subtree; `booking/calendar/sidebar`
  cited only the one document under it
- the scoped repeat came back **CACHED**; the same question unscoped bought a
  completion, as did the same question in a sibling folder
- History showed three rows for two similar questions — two scoped (with their folder
  on the badge), one not — and replaying a scoped row restored the folder *and* was
  free
- `<nes-tree>` rows are 44px on a phone, the path down to a restored scope starts
  expanded, siblings stay collapsed, no console errors, no overflow at 390px
- the scope bar needed its own opaque background: the dock fades in from transparent,
  so a row added at its top sat over the tree behind it

## Docs synced in the same commit

Guide (habit 3 is now "check what is indexed — and narrow to it", plus two capability
rows), Dev page (`scope` on the HTTP surface, the two invariants above with their
test names), Deploy page and `.env.example` (`CONTEXT_WINDOW`, `PRICE_IN`,
`PRICE_OUT`, which arrived with the status line and were undocumented), README (HTTP
table, which had drifted — it was missing tickets, documents and history, and still
described `/api/health` as `{"ok":true}`), CLAUDE.md (a fifth invariant).

## Still open

- **The corpus repo has no remote** — unchanged, and still the first thing to do
  (§3a of the deployment entry). It must be a *private* repo; this one is public.
- **A scope is not a permission.** It filters retrieval for someone who chose it; it
  hides nothing from anyone who does not. Per-document access control remains out of
  scope, and the guide says so in the same words it always did.
