# 2026-08-03 — seven requests, four built, three refused

Seven improvements were asked for in one go. Three of them were already shipped, in whole or
in part, and re-deriving that costs an afternoon each time — so what they are, and why the
refusals are refusals, is written down here rather than left to the next reading of the code.

## Built

| # | What | Where |
|---|---|---|
| 3 | A question about the library, answered from its rows | `internal/rag/smalltalk.go`, `internal/db/store.go`, `ba.html#recent` |
| 7 | Tick several documents and remove them together | `composables/library.js`, `LibraryPanel.vue`, `ba.html#import` |
| 2 | The noise a pasted document arrives with, stripped on the way to the index | `internal/rag/chunk.go`, `ba.html#import` |
| 4 | A seeded `kind` vocabulary, and two fields derived from the path | `lib/upload.js`, `composables/library.js`, `ba.html#import` |

## Refused, and why

### #1 — rewriting the user's question before retrieval

**It already exists, and only where it pays.** `Engine.standalone` (`rag.go`, `memory.go`)
rewrites a *follow-up* against the thread, because a pronoun embeds to nothing.
`db.normalise` (`tickets.go`) folds case and trailing punctuation so that "thanh toán thế
nào", "…nào?" and "…nào???" are one cache key and one ticket.

What was asked for beyond that is rewriting the **first** question of every conversation, and
it loses twice:

- it is **one completion on every cold question**, paid before retrieval starts, on the path
  that is supposed to be the cheap one;
- a model is not deterministic, so the same question rewrites two ways and the cache key
  stops being a key. The saving the cache exists for goes with it.

The existing design — rewrite only when the question cannot stand alone — is the version that
survives both.

### #5 — emoji in the system prompt

**It is already there.** A diagram node opens with one (👆 ⚙️ ❓ ✅ ⛔ 📄), a repeated column
in an enumeration table is a glyph with a legend under it (✅ ⛔ ⏸ ⚠️ ❓), and six alert panels
carry their own colour.

The part worth knowing before anyone adds more: **the prompt's hash is inside `Engine.sig`**,
so editing one character of it invalidates every cached answer in the database. That is
correct — an answer produced under different rules is a different answer — but it means a
prompt edit is never free. If more emoji is wanted, it rides along with the next prompt change
rather than buying a corpus-wide re-answer of its own.

### #6 — selecting several documents *to ask across*

Not the same thing as #7, and this is the trap: one word, two orders of cost.

- `scope` is **half the answer cache's key**, and it is one prefix constraining **both**
  retrievers before they rank (invariant 4). A set of documents means a key that has to be
  order-stable — or `"a,b"` and `"b,a"` become two cached answers to one question — and it
  means rewriting the `IN (…)` filter on both legs.
- Tapping a folder to scope a question **already ships**: `CorpusTree.vue` emits `pick`, and
  a folder's value is its own path.

So: folder scope stays, arbitrary multi-select does not get built. **#7 was accepted** because
it is the opposite shape — N calls to an endpoint that already exists, idempotent, gated, with
no server change at all. If a later session reads "multi-select was refused" and reaches for
the bulk remove, this is the paragraph that says don't.

## Two decisions inside the work

**#2 cleans chunks, never the body.** `documents.body` is what a BA typed and vouched for
(invariant 1), and it is what they see when they open the document again. The chunks are
derived and any re-index rebuilds them, so that is where a cleaner belongs. Fenced code blocks
are exempt from everything but line endings: a trailing space or an `<!-- -->` inside a fence
is the thing being demonstrated.

Consequence, stated rather than discovered later: documents indexed before this keep the
sections they have until they are saved again or re-ingested.

**#3 reads the store, and that is not the thing `smalltalk.go` refuses.** That file says
*"Nothing here claims anything about the corpus's **contents** — … a sentence **invented** here
could disagree with it."* The distinction is invented versus measured. A recency answer is the
same rows the library screen renders, read at the same moment, so there is nothing for it to
disagree with. It is placed above the cache for the same reason it exists: an answer about
what is newest is the one answer that goes stale by sitting still.

## A pre-existing bug the work surfaced

`class WrongPass extends Error {}` does not set `name` — an instance reports `"Error"`. Three
`e.name === 'WrongPass'` tests in `composables/library.js` had therefore been dead since it was
written: a refused password while **saving or removing a single document** put the server's
sentence in the panel's error line instead of taking the screen back to the unlock form, and
the BA was left looking at a write surface that could no longer write.

Found by driving the running product with the password changed underneath it, while checking
that a bulk removal stops on the first refusal instead of repeating it down the list. One line
in `lib/qa.js` fixes all three call sites.
