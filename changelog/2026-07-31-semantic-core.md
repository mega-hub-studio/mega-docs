# 2026-07-31 — The answer got as wide as the model, and gained a third tier

Five changes that are really one: the engine was paying for a reader and using a skim.
`TOP_K` was six sections — about 3.5k tokens against a 128k window, three per cent — and
every other limitation followed from nobody having a number to compare against.

## What shipped

| | before | now |
|---|---|---|
| sections per answer | `TOP_K`, fixed at 6 | as many as `CONTEXT_SHARE` (0.5) of the picked model's window holds. `TOP_K` is the floor, and the whole rule when no window is declared |
| breadth | six chunks could be six chunks of one file | `maxPerDoc = 3`, always on, cost-neutral |
| depth | the 2400-char cut that happened to rank | that chunk plus its `ord` neighbours, when a budget exists |
| outside knowledge | forbidden — *"Answer ONLY from the CONTEXT"* | three tiers: `[n]` documents, `[wN]` public search, one `[!GENERAL]` panel |
| thread | 3 turns, client-side | 12 offered, server trims to `THREAD_SHARE` of the real window |
| visibility | nothing | `sections/candidates` on the status line, beside `kept/offered` |

## The decisions the next session would otherwise re-derive

### The automatic trigger was built twice, measured twice, and deleted

The supplement was meant to fire "when the documents only partly answered". Two versions tried
to measure that and **both fired on essentially every question**. Neither was caught by review;
both were caught by pointing them at the real instance.

| version | the rule | what it measured against | why it was wrong |
|---|---|---|---|
| 1 | `chars < budget × 0.5` | half of `CONTEXT_SHARE` of the model's window | against `gpt-4o-mini:128000` that is **128,000 characters**, ~53 full chunks. It measured *how big the model is* when the question was *how much the documents had* |
| 2 | `Offered < askFor(model)` | the candidate pool | `maxPerDoc` caps a **13-document** corpus at 39 sections against a pool of **40**. Production was one document short of never firing — a boundary that is an accident of two unrelated constants |

So it is a **reader's switch** now: a `.switch` in the settings drawer, off by default, rendered
only when `/api/health` reports `search: true`, travelling with the question the way the model
pick does. Two switches and no guessing — the operator decides whether this instance *can*, the
reader decides whether this question *should*.

That deleted `thinShare`, `thin()`, `corpusRanOut()` and `searchSig()`. What survives is the one
gate that was never a heuristic: `len(hits) == 0` returns the no-answer sentence before any
provider is reached, so a gap still goes to a BA and never to a search API.

Also rejected on the way through: a similarity threshold on the top hit's distance (absolute
embedding distances are model-dependent, so the threshold is a knob nobody knows how to turn),
and a threshold on the fused RRF score (rank-based and genuinely model-independent, but the
approved-chunk `×1.2` boost perturbs it exactly across the single-leg/both-legs boundary).

**The cache half inverted with it.** An automatic trigger was startup config, so it belonged in
the *signature*. A reader's tick is a per-request choice, so it belongs in the **key**, beside
the scope and the model — the signature would prune every no-web answer the moment one reader
ticked the box. `cacheKey` is four fields now; rows written under three age out, which is the
same trade taken when the model joined it.

This is where `changelog/2026-07-28-memory-and-external-search.md` and invariant 3 finally agree
rather than being reconciled: that entry's *"off unless switched on, and visible when it is on"*
is exactly a reader toggle, and invariant 3's key/signature split then follows without argument.
The two intermediate designs were the detour.

### One defect the prompt could not fix

With the supplement on, a real answer came back offering three readings of "what is a
goroutine" — every one of them cited `[w1]`, `[w2]`, `[w3]`: a YouTube video and two blog posts.
Picking a reading sends it back as the next question, where the corpus cannot answer it, so the
clarify card was a menu of guaranteed misses.

A prompt rule forbidding it was written, ignored, strengthened, and ignored again. The check is
in code now — `grounded()` in `web/ui/src/lib/answer.js` drops an option whose citations are
*all* web, and a card with nothing left to pick is not rendered at all. An option carrying no
citation is kept: the model does not always cite a reading, and refusing those would empty most
cards.

**And the other half of it, which the prompt could not fix either.** The same model, asked
something the corpus did not cover, wrote a `[!QUESTION]` checklist and put the no-answer
sentence *underneath* it — forbidden in two places in the prompt, written twice anyway. Under
`isMiss`'s exact equality that reply is not a miss, so a **gap was cached as an answer**, with
sources printed under "this is not in the documents" and Ask BA no longer the only move left.

`isMiss` is now the sentence at the **end** plus **no `[n]` anywhere**:

```go
func isMiss(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasSuffix(s, NoAnswer) && !citeMarker.MatchString(s)
}
```

Both halves are load-bearing, and `cachepolicy_test.go` had already pinned the case that proves
it: `"Folders are kebab-case [1].\n\nLeave policy: " + NoAnswer` must stay cacheable. A bare
`HasSuffix` breaks it, which is why exactness existed — the citation test is what keeps that
protection while closing the hole. `[wN]` deliberately does not count: a reply whose only sources
are public pages is precisely a question the documents did not answer.

What settles the remaining doubt is the asymmetry, and it is worth writing down because it
generalises: **a reply misread as a miss costs one completion the next time it is asked; a gap
misread as an answer is cached until the corpus changes and hides itself from the loop that
would have fixed it.** When a classifier has to be wrong somewhere, be wrong in the direction
that costs money rather than the one that costs the corpus.

Confirmed red first: under `s == NoAnswer` the two new cases report
`isMiss = false, want true`.

### Why the model's window went in the signature and not the key

`contextSig()` is `contextShare + fmt.Sprint(e.models)`, so widening **one** model's window
invalidates **every** model's rows. Deliberate over-invalidation: it costs one cold cache
after an admin edit nobody makes twice a day, and it buys leaving `db.cacheKey`,
`keyParts` and `Cached` completely alone. Putting the window in the key was the precise
option and cost surgery in three functions for accuracy that matters only when someone
edits one model's window and nothing else.

### `referencedWeb` has the opposite fallback to `referenced`, on purpose

An answer citing no document keeps the whole document list — the sources are the only
provenance a reader has. An answer citing no `[wN]` shows **none**: printing links under an
answer that did not use them claims a supplement that never happened. Two twelve-line
functions rather than one parameterised by a regex, because the *rule* differs, not just the
pattern.

### Rejected: multi-query / HyDE fusion

`fuse()` is already variadic and `Embed([]string)` already batches, so the retrieval code
would be nearly free. It needs a completion to produce the variant on **every fresh
question** — today `standalone()` only runs for follow-ups — so it doubles latency on the
main path for a benefit nobody has measured. The seam stays free for the day there is an
eval. Same reasoning that kept the history hash out of the cache key.

## Things worth knowing about the implementation

- **`Recall` is one type with two uses**, and its doc comment now says so: `kept/offered`
  for the thread, `sections/candidates` for the corpus. A second struct for one pair of ints
  would be a second thing to keep in step.
- **`pool = 40` became `db.CandidatePool`**, exported, because the engine now asks for
  exactly that many when it is filling a budget. Two numbers for "as wide as retrieval goes"
  would disagree the first time one moved.
- **`stitchNeighbours` never reorders or renumbers.** Same hit count, same `ChunkID`, same
  order — only `Content` grows — so `referenced()` and the `[n]` markers needed no change at
  all. A neighbour that is already a hit is skipped, or two adjacent hits would print the
  same paragraph under two citations.
- **`(document_id, ord) IN ((?,?),…)`** is valid SQLite row-value syntax; checked against
  3.45 before it was written, both with and without the `VALUES` keyword.
- **`.explain { --accent: var(--teal) }`** is four lines in `styles.css`. 0.15.0 aliases six
  accent tokens to `.callout` classes (info · tip · memo · warn · gotcha · quest) and this
  app already means something by all six; `--teal` is one of the six raw tokens it ships and
  aliases to nothing. Verified in the pinned tarball's `tokens.css`, not guessed.
- **`maxAsk` went 64 KiB → 256 KiB** with `RECALL_TURNS` 3 → 12. Twelve of the long answers
  this engine writes clear 64 KiB, and a refused request is a failed question rather than a
  shorter memory. Which turns to send is still the client's; how much of them the model
  reads is still `THREAD_SHARE`, measured against the real window.
- **A `/g` regex reused with `.test()` carries `lastIndex`.** `linkCites` matches both
  markers with one pattern now, and the walk's cheap pre-check is a separate non-global
  literal for exactly this reason — sharing it would have made every other text node report
  no match.

## Not done, and why

- **No rolling summary of the dropped prefix of a thread.** `replay()` already reports
  `kept/offered` to the status line, so a trim is visible rather than silent, and one extra
  completion per long turn is a cost nobody has measured.
- **No `SEARCH_MAX_RESULTS`.** `webTopK = 3`, a constant. Rule 20 — nobody has asked.
- **No per-document cap knob.** `maxPerDoc = 3`, same reason.
- **`web/retrieval.mmd` unchanged**, for the reason `2026-07-29-conversation-memory.md`
  already gave: the diagram is deliberately shallow so a spotlit node fits a phone viewport.
  The budget, the cap and the stitch are in the section's own prose and its `datalist`.
- **No cost line for the search API.** `Usage` is provider tokens; a flat or metered
  third-party fee is a different kind of number and nobody has asked to see it.

## Verification

`make lint` **0 issues**, `make lint-js` clean, `go test` green across `cmd/…`,
`internal/…` and `web` — including the guide joins, which went red first exactly as rule 15
describes (`CONTEXT_SHARE` declared by no section, then `SEARCH_API_KEY`/`SEARCH_BASE_URL`,
then a `data-test` naming a test that did not exist yet).

`TestRetrievalWidensToTheModelsWindow` was confirmed **red** before it was green: with
`contextBudget` forced to 0 it reports `read 3 sections; want more than TOP_K's 3`.

Four `retiredClaims` entries were added, in both languages: the guide's *"answer **only**
from those sections"* and the three places `TOP_K` was named as the number that decides how
much an answer reads.
