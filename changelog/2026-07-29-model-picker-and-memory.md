# 2026-07-29 — A reader picks the model, the thread is trimmed to it, and one gear holds both

Four things were asked for: a model switcher, a smarter prompt, asking back when the question
is unclear, and session memory sized to the context window. **Two of the four were already
built** and reading the code first is what kept this from being twice the change:

| asked for | found |
|---|---|
| optimise the user's prompt | `rag/memory.go` already rewrites every follow-up into a standalone question *before* retrieval, and falls back to the typed words when the rewrite fails |
| ask back when unclear | `rag/smalltalk.go` already answers a vague question with the corpus's own vocabulary, and the grounding prompt already requires both readings plus a citation each when a question is ambiguous |
| memory sized to the window | half: the thread reached the model, but the client cut it at a hardcoded `RECALL_TURNS = 3` with no relation to any window |
| model switcher | not there: one `CHAT_MODEL`, and the model baked into the cache *signature* |

So the work was the two gaps, and the surface that makes them visible.

## The model moved from the signature to the key, and that is the whole design

`Engine.sig()` was `corpus|chatModel|prompt`, and `Store.Cache` prunes every row whose
signature differs. With one model per instance that was invisible. With a picker it would mean
**every switch throws the other model's answers away** — which is, word for word, the reason
CLAUDE.md already gives for keeping the *scope* out of the signature. Same trap, second door.

The key is now `model \x1f scope \x1f normalised-question`, three fields always, and the
signature is what an answer was produced *under* (corpus + prompt) while the key is what it was
produced *for*. `TestAnotherModelIsAnotherAnswerAndBothSurvive` is the assertion that matters:
asking on the cheap model, then the strong one, then the cheap one again — and the third ask is
still free. Rows written before the model joined the key stop matching and age out through the
LRU; one cold cache is cheaper than a migration for a derived table.

## `CHAT_MODELS`: one knob replaces four, without breaking a running instance

```
CHAT_MODELS=gpt-4o-mini:128000:0.15:0.60,gpt-4o:128000:2.50:10.00
```

First entry is the default; window and prices are optional per entry and an entry without them
simply prints no percentage and no cost — the existing "unmeasured is not zero" rule, now per
model. **Unset means the single-model instance**: `CHAT_MODEL` + `CONTEXT_WINDOW` + `PRICE_IN` +
`PRICE_OUT` become the one entry, so the tailnet instance did not need reconfiguring and no
fact is written twice. A malformed list is `log.Fatal` at startup, deliberately: falling back
to a default answers questions with something the operator did not choose, and dropping a bad
entry hides a typo in what is also a spending allowlist.

**The client names a model and the server never trusts it.** `pick()` refuses anything outside
the list with a 400 rather than substituting the default, because a reader who chose the strong
model and silently got the cheap one reads the answer as that model's best effort.

## Memory: the budget is the model's, and it is on screen

`replay()` now takes the window and keeps turns newest-first until they would crowd out the
retrieved sections — `threadShare = 0.35`, with the other two thirds left for the documents and
the completion, because a thread that squeezes them buys memory by making the answer worse. The
estimate is `len/4` characters-per-token and **not** a tokenizer: a dependency and a per-model
table for a number that only decides where to cut a list, when the provider reports the true
count in its usage frame moments later. Four is low for prose, so the budget errs toward keeping
fewer turns — the failure that costs nothing.

The client now offers 12 turns instead of cutting at 3, and the count that survived rides back
on the `done` frame. The status line prints `3/8`. That is not decoration: a silent trim and an
assistant that forgot are indistinguishable from the outside, which is exactly how "it has
memory" becomes a claim nobody can check.

## One gear, one panel — and `/#/admin` stayed

The plan said delete the admin screen for a single settings surface. Reading it first changed
that: it is **three tabs** (Settings · Runtime · Usage) of read-only instance diagnostics behind
`ADMIN_PASS`, and folding that into a 22rem drawer would make the drawer a screen. The honest
split is not by subject, it is **by who decides**:

- the drawer holds what a *reader* chooses, kept in this browser — model, language, sound
- `/#/admin` holds what an *operator* configured, read from `.env` at startup

So the drawer links to it instead of restating it, and the row is absent entirely when the
instance has no admin secret. Deleting nothing also cost nothing: no guide rewrite, no
`retiredClaims`, no lost diagnostics.

The panel is the library's `.drawer` on a native `<dialog>` — `showModal()`, so Escape, the
focus trap and the backdrop are the platform's — with `.eyebrow` sections, `.field`/`.select`
for the model, `.segment` for the language, `.switch` for the sound, `.datalist` for the
read-only numbers. The language button left the bar for it: a control used twice a year was
taking a quarter of a 390px bar.

The strip's model item became the *door* to the panel rather than a second picker: one place to
change it, one place to read it. `ChatTurn` carries a `.badge` naming the model that answered,
which is what makes "regenerate this with the stronger one" a comparison instead of a guess —
Regenerate sends the pick as it is *now*, on purpose.

## Two traps this cost, both already written down somewhere

1. **`display` on a `<dialog>` must be qualified by `[open]`.** `styles.css` documents this
   three lines above where I broke it — an unqualified `.drawer { display: flex }` beats the
   UA's `dialog:not([open]) { display: none }` because the cascade compares origin before
   specificity, and the panel sat on screen as a 351×844 box with `open` false. Worse, the fix
   *split across two rules* was folded back together by the minifier —
   `.drawer[open]{display:flex}` beside `.drawer{flex-direction:column}` came out as one
   `.drawer{flex-direction:column;display:flex}`. Both declarations now live in the same
   `[open]` rule and there is no unqualified `.drawer` rule at all.
2. **`nes-icon name="history"` does not exist in 0.13.0** and renders an empty box in silence,
   which is the trap the strip's own comment already names about `name="branch"`. `icons.d.ts`
   in the pinned package is the only thing that answers it; the memory item uses `chat`.

## Open, for upstream

`.drawer` anchors itself with `inset-inline-end: 0` and leaves the start inset alone. On a
`<dialog>` the UA sheet has already set `inset-inline: 0`, so with the recipe's own `margin: 0`
the box is over-constrained and the *start* wins: the panel opens against the left edge, and
`.drawer.start` renders identically — which is the tell. One local override
(`inset-inline-start: auto`) until a release qualifies it.
