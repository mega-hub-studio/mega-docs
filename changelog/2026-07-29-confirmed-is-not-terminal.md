# `confirmed` stops being a trap, and four UI faults behind it

Two sessions' worth of complaints turned out to share one shape: a thing the product
could get *into* and not out of.

## The bug that started it

A BA reported "I can't delete a QA". They were right, and the state on the deployed
instance said exactly why:

```
documents  id=7  qa/ticket-1.md   deleted_at = 2026-07-29 05:23:38   ← they DID delete it
tickets    id=1  status=confirmed doc_path   = qa/ticket-1.md        ← unchanged since 27 Jul
```

The document removal worked. Nothing else did:

1. `Draft`, `Confirm` and `Reject` all carried `WHERE status IN ('open','answered')`, so
   every transition out of `confirmed` returned "already confirmed" → **409**.
2. There was no `Delete` on the `Knowledge` interface and no `DELETE /api/tickets/{id}`.
3. `rag.Remove` never touched the ticket, so removing the document left the queue
   asserting `IN KNOWLEDGE` and naming a document that answered nothing.
4. `TicketCard.vue` rendered the confirmed state as a read-only `<dl>` — no button, so
   even a working API would not have been reachable.

`confirmed` was reachable only by doing the work, and it was the one state with no exit.

## What replaced it

No new state — only the edges the old one lacked:

```
open ──draft──▶ answered ──confirm──▶ confirmed ──confirm──▶ confirmed  (republish)
                    ▲                     │
                    └────── retract ──────┘
open · answered · confirmed ──reject──▶ rejected
any state ──delete──▶ gone
```

`Reject`, `Retract` and `Delete` all call `unpublish` first, so the ticket and the corpus
can never disagree about whether an answer is live. Re-confirming republishes to the same
`qa/ticket-N.md`, which is what makes correcting a published answer one action instead of
a retract the BA has to remember to undo.

**Delete is safe, and that is not a judgement call.** `RemoveDocument` is soft, so the
answer's *text* survives as a row with a `deleted_at`. Deleting a ticket costs the
question and its history, never the words somebody wrote. That is the whole reason it
was acceptable to offer at all.

`TestConfirmingTwiceIsRefused` was inverted rather than deleted — it is
`TestConfirmingTwiceCorrectsTheAnswerInPlace` now, and its comment records what the old
rule bought (nothing) and cost (a permanent mistake).

## The UI half

Four separate faults, and each one is worth reading as its own lesson.

**The library form did not come to the person who opened it.** `edit()` set `open = true`
and stopped. The form renders below the list, below the import card, on a screen that
also carries the queue — so pressing EDIT on a phone scrolled nothing, focused nothing,
and looked exactly like a dead button. `reveal()` scrolls and focuses; the head sticks so
you cannot lose track of which document you are overwriting. It stayed **inline rather
than becoming a modal** — the recorded reason (a BA writes while reading the list) is
still true, and it is also why nothing dims the list.

**Every toast in the app printed its own markup.** `toast(msg)` renders `msg` as *text*;
markup needs `{html: true}`. Thirteen call sites passed `<b>…</b>` without it, so the tags
appeared on screen. Fixed with the library's own `title` option, which is the recipe for
that exact shape and cannot inject the way an interpolated `html: true` could. Two
adjacent bugs fell out: `library.js` passed `'gold'` as the *options object*, so the
second argument was a string, no `accent` was read off it, and every toast there landed
on the default colour — and `importer.js` still branched on `r.trash`, a field the server
stopped returning when removal became a column.

**The release dialog showed nothing on iOS.** `.rel-body` was `flex: 1` — a zero basis —
inside a dialog whose own height is `auto`, so the body contributed nothing to the height
it was being sized against. Chrome resolves that from the max-content contribution;
WebKit does not, so the phone rendered a title bar and no notes. `flex: 1 1 auto` is the
fix. **Verified in Chrome only** (no regression: identical layout both ways) — the iOS
reproduction is unconfirmed, because the bug does not occur in the engine available here.

**The diagram viewer fought its own drag.** `.mermaid-view` is the library's diagram
*frame*: bordered, padded, and `overflow: auto`. Nested inside `.zoom-view`, which sets
`touch-action: none` precisely so it can own the drag, that made one gesture with two
handlers — the panner translating the stage while the element inside it scrolled. It also
drew a third concentric border inside the dialog's and the panner's. Scoped away inside
the viewer only; the in-answer diagram keeps the frame it is written for.

## Also retired (rule 26)

The new `retiredClaims` entry for `docs/qa/` caught two pages nobody was looking at:
`deploy.html` told operators to review published answers with `git diff docs/qa/`, and
`docs.html` had a callout headed *"Why a confirm writes a file"* asserting that **"the
database stays disposable"** — the exact inversion of invariant 1, on the page an
operator reads before deciding what to back up. Both are gone. The prefix was dead too:
`rag.QADir` is `"qa"`, so `docs/qa/…` resolved to nothing, and the app's own ticket form
was printing it.

## Open

- The BA screen is English-only while the guide is bilingual. `lib/i18n.js` covers `app`,
  `empty` and `release` and nothing else, so the new ticket controls are English by
  consistency, not by decision. Translating half the screen would be worse than either
  end state; doing the whole BA screen is its own change.
- The dock collapse is deliberately **not remembered** across loads — a collapsed dock
  restored on open is a reader who cannot find the way to ask anything.
