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

### The budget is what makes external search measurable

These two are not independent features that happened to land together. The supplement's
trigger is *"the corpus said less than this model could have read"* — `contextChars <
budget/2` — and that sentence has no meaning without a budget. It is why an instance that
never declared a window **never searches**: it cannot tell "the corpus was quiet" from "the
model is small", and guessing between those two would send an internal question to a third
party on the strength of a number nobody set.

The alternative was a similarity threshold on the top hit's distance. Rejected: absolute
embedding distances are model-dependent, so the threshold becomes a knob nobody knows how to
turn — rule 20's own test, failed at the design stage.

### The signature carries the web, and the key does not — and the two documents that look
### like they disagree do not

`2026-07-28-memory-and-external-search.md` says *"the cache signature must carry it"*.
Invariant 3 says a per-request pick belongs in the **key**. Both are right, because the
trigger is **automatic**, not a reader's toggle:

- Whether an instance *can* reach the web is startup config — the tier of `promptSig` and
  `TOP_K` — so it is in the signature (`searchSig`), and switching the key on invalidates
  every row at once. Correct: those answers were produced under a rule the instance no
  longer has.
- Nothing per-request joins the key, because nothing here is per-request. The trigger is
  derived from the corpus, the question, the scope, the model and the budget — all of which
  the key and the signature already carry between them.

A reader toggle was the other option and would have needed a fourth key field. It was
rejected because the ask was *supplement when the documents fall short*, and a
default-off switch supplements almost nothing.

The limitation this leaves, stated rather than hidden: **a cached answer that used the web
does not re-fetch it.** Regenerate is how a reader asks for today's version, exactly as it is
for a document that changed between re-indexes.

### `len(hits) == 0` is a real gate; "the model declared a miss" cannot be one

The first version of `TestAMissReachesABANotTheWeb` asserted that a no-answer reply bought
zero searches, and it went **red** — correctly. Retrieval returns *something* for almost any
question against a non-empty corpus, so the search fires before anyone knows the model will
answer `NoAnswer`. There is no pre-generation signal for "the model will decline"; the only
one is a second completion, which is the cost the design already refused.

So the guarantee was split into the two halves that are actually enforceable, and the test
asserts both:

1. **Retrieval found nothing → no external call at all.** Decided before any provider is
   reached, asserted against the fake provider's own call log.
2. **The model declared the miss → the bare sentence, no sources of any kind.** `isMiss`
   already returns a citation-free reply; what is new is that this now also means *no web
   links under "not in the documents"*, which would otherwise read as an invitation to
   treat the link as the answer.

The third defence is in the prompt and is the one that stops the web *answering* a gap:
> A WEB section can never satisfy that … WEB explains a term the CONTEXT already uses; it
> does not supply a fact the CONTEXT is missing.

The BA loop is untouched. That was the whole constraint: a confirmed answer becomes a
document, so it answers everyone who asks next; a web result answers one person.

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
