# 2026-07-30 — An alert with no content, a badge that squeezed its own label, and a queue that was only mislabelled

Three reported from a phone. One of them turned out not to be a bug in the code at all, which is
the part worth keeping.

## `IN KNOWLEDGE` in the "waiting" list — the label was the bug

Reported as: *why is a ticket the BA already resolved still in the waiting list?* Traced the whole
chain first, because a filter is the obvious fix and it would have been wrong:

- `db.Queue` has **no WHERE clause** on either query — it returns all four statuses on purpose
  ("the whole BA view in one response") and counts each status separately.
- `qa.queue()` → `useQaLoop.refresh()` → `App.vue` → `AskScreen` → `EmptyScreen` passes it through
  untouched. There is no filter anywhere, by design.
- `EmptyScreen.vue`'s own comment says why: *"so a gap doesn't get filed twice and the person who
  filed it can see it land."* Seeing it land **is** seeing `IN KNOWLEDGE`.

So the list is right and the English is right — `empty.withBa` reads *"Questions with a BA (n)"*,
which promises exactly what it shows. The **Vietnamese** read *"Câu hỏi đang chờ BA"*, which
promises a queue of things still waiting. One word of translation, one bug report.

`i18n.js` now says *"Câu hỏi đã gửi BA ({n})"* — sent to, not waiting on. The block's comment
carries the report and the instruction not to answer it by filtering, because the next agent will
reach for the filter too.

**The lesson is the order:** trace to the decision before writing the fix. The filter would have
deleted a feature to satisfy a mistranslation, and `make check` would have been green.

## An empty bordered box with its content stranded underneath

`dressAlerts` strips the `[!KIND]` marker out of a blockquote's first text node, removes the node
if nothing is left of it — and then built the `.callout` panel **unconditionally**. Nothing asked
whether the quote still held anything.

Two ways in, and both shipped:

1. The model writes `> [!NOTE]` and puts the prose in a **sibling** block instead of the quote.
   marked closes the blockquote at the end of that line, so the quote holds only the marker; the
   list or paragraph that follows is a separate top-level element. Panel: empty. Content: below it.
2. **Every alert, mid-stream.** `turnHtml` renders on every chunk, so between the marker arriving
   and its first word there is always a frame where the quote is marker-only. That is the empty
   box flashing on screen for every `[!NOTE]` the model has ever written.

Four lines, right after the marker is stripped: if the quote has no text left, remove it and move
on. Removing rather than keeping an empty one is what makes case 2 read right — the panel appears
with its first character instead of flashing an empty frame first, which is still what
`answer.js`'s own comment asks for (*"the panel renders as the question is written"*). An alert
with no content is not an alert.

Not guarded, deliberately: an alert whose only content is an image would also be dropped, because
the test is `textContent`. Nobody has produced one, and `!trim() && !querySelector('img,svg,pre,table')`
is code paid for a future that has not arrived (rule 20). One reported case changes that.

## `RECOMMENDED` → a star

`.check` is the library's `inline-flex` label with **no wrap**, and `.checkbox` carries
`flex: none` — so the option text is the only child that can shrink. A 90px uppercase mono chip
therefore took 90px out of the sentence it was labelling, and on a phone the question broke into a
three-word column beside a badge nobody needs to read twice.

`<nes-icon name="star">` says the same thing in the width of one character. `star` is in the
pinned `icons.d.ts` — a name the release does not have renders an empty box and says nothing, so
that check is not optional. `role="img"` and `aria-label` because the library renders the svg
`aria-hidden`: without them the mark is invisible to a screen reader, and `title` alone is not
reliably announced, so both attributes are there and each does a different job.

**No guide sync needed, and that was checked rather than assumed.** No published page names the
badge; `docs.html`'s *"the likeliest is ticked already"* describes the pre-tick, which this does
not touch — `opt.recommended` still drives `:checked`. Only the mark's form changed, so nothing
went into `retiredClaims`.

## Still owed

`ClarifyCard.vue` hardcodes two English strings — the new `Recommended` and the existing
`ASK THIS` — while the app has `useT()` as its only i18n door. Consistent inside the file and
wrong for the app; it is two keys (`clarify.recommended`, `clarify.ask`) whenever someone touches
that file next, not a reason to widen this change.
