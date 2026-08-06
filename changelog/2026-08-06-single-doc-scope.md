# 2026-08-06 — A scope holding one document read three sections of forty-three

Reported from the deployed instance: `Naver Booking Action Matrix đầy đủ như thế nào?`, scope
`AHA/FAQ/Naver`, answered **"Không tìm thấy thông tin này trong tài liệu."** — while the
corpus held the complete matrix, correctly chunked, correctly embedded and ranked #2 by BM25
inside that scope.

## The cause is one line, and its premise was a comment

`capPerDoc` ran unconditionally in `hits()`. Every candidate in a single-document scope shares
one `DocPath`, so the cap kept three and dropped forty. Traced against the deployed `.env`
(`CHAT_MODELS=gpt-4o-mini:128000:…`, `TOP_K=6`, `CONTEXT_SHARE` at its 0.5 default):

```
contextBudget = int(128000 × 0.5) × 4   = 256 000 chars     retrieve.go
k             = db.CandidatePool        = 40                retrieve.go   (budget > 0)
fuse (RRF)                              → ~43 ids           store.go
capPerDoc                               → 3   ◀── binds     store.go
len(hits) > k                           → 3 > 40? no        store.go      (k never binds)
trimToBudget                            → 3 chunks ≈ 4 KB   retrieve.go   (budget never binds)
```

Seven per cent of the scope, against a budget that holds the whole 29 989-character document
eight times over. The cap's own doc comment had already stated the premise that fails:

> *"The cap costs nothing: the slots freed go to the next-best chunk of some other document."*

In a scope holding one document there is no other document. The freed slots go nowhere, so a
rule for **spreading** an answer became a rule for **cutting** it. The condition was real all
along; it was written as reassurance instead of as code.

## What was measured, on the running instance

Six requests, `fresh:true`, same scope. **Every one reported `sections:3, candidates:3`** —
the cap binding at 3 of 43, live, in production. What differed was only whether the chunk
holding the matrix (ord 9) won one of the three slots:

| request | Q10 in the three? | reply |
|---|---|---|
| the exact question, ×2 | yes | the full table |
| paraphrase *"Cho tôi bảng Action Matrix đầy đủ…"* | yes | the full table |
| paraphrase *"Liệt kê toàn bộ action matrix…"* | **no** | the table anyway — it arrived stitched into hit `[2]` as an `ord` neighbour, cited under the wrong heading |
| the exact question, in a thread, ×2 | yes | a `[!QUESTION]` disambiguation instead of an answer |

Three behaviours for one question, decided by which three of forty-three sections survived.
The reported reply is the fourth face of the same die, and it is **not reproduced here** — six
attempts did not produce the sentinel verbatim. What is proven is the mechanism that makes the
outcome a coin flip; the sentinel is what the prompt correctly emits when the flip loses.

Worth keeping for the next reader: `stitchNeighbours` was quietly compensating. A chunk that
lost its own slot could still reach the model as `ord±1` of a chunk that won one — which is
why the failure looked intermittent rather than structural, and why the citation then names
the neighbour instead of the section the answer came from.

## The fix

`capPerDoc` counts distinct document paths first and returns untouched when there are fewer
than two. Six lines, no signature change, no new knob:

- **Nothing changes for a corpus-wide question.** Two or more documents in the ranked set →
  the original branch, unchanged. `TestOneDocumentCannotFillTheAnswer`'s three-document
  fixture asserts exactly that, still.
- **`TOP_K` and `CONTEXT_SHARE` get their jobs back.** They were written to decide retrieval
  width and could not, because the cap ran ahead of both. The scope above now reads 40 of 43.

## The regression gap this had to close in the same commit

Both existing tests **passed silently** against the fixed code before a new assertion existed:
`TestOneDocumentCannotFillTheAnswer` searches unscoped over three documents, so the
single-document branch is never reached; `TestScopedSearchRanksWithinTheScope` calls
`Search(..., k=3, ...)`, and at `k == maxPerDoc == 3` the k-truncation produces the same
three hits either way. A fix nothing can fail on is a fix that comes back.

So the assertion went into the test that already owns the cap (rule 21), on the fixture that
was already there, and red-green was **run** rather than assumed: with the fix stashed it
fails with *"a scope holding one document read 3 of its 10 sections"*; restored, it passes.

## Rejected

- **A `MAX_PER_DOC` knob.** Rule 20 — nobody turns it, and a knob costs a documented section.
  The bug was a missing condition, not a missing number.
- **Two-pass backfill** (cap first, then refill spare slots from the leftovers). It changes
  wide-corpus behaviour, which is the case the cap was written for. `len(docs) < 2` touches
  nothing that works today.
- **Widening `toFTSQuery` / stripping Vietnamese stopwords.** Measured against the live index:
  the target chunk ranks #2 of 39 inside the scope. Keyword retrieval was never the bottleneck.
- **Rewriting `systemPrompt` for a small model.** `promptSig` is in the cache signature
  (rule 3), so editing it discards every cached answer — too big a bill for a hypothesis that
  had not been separated from the retrieval defect yet. If the instability survives this
  change, the cheaper lever is the model: `CHAT_MODELS`' first entry is the default, and
  `gpt-4o` is already in the deployed list. The chat model lives in the cache *key*, not the
  signature, so reordering that line costs no cached answer at all — which is what rule 3's
  split is for.
- **A retrieval log or a debug endpoint.** The SSE `done` frame already carries
  `sections/candidates`, and that pair is what measured all six requests above.

## Still open

`statusline.js` hides `sections/candidates` whenever the two agree, so `3/3` — the shape this
bug always took — displayed nothing at all. The pair reports what a *budget* trimmed, and the
cap was never a budget. Not changed here: after this fix the same scope reports `40/40` and
still shows nothing, which is now honest (nothing was dropped) but says nothing either. Left
for whoever next needs to see how much of a scope an answer actually read.
