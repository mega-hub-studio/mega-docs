# 2026-07-30 — the reply that asks back, and caveats as panels

An ambiguous question was already detected — `systemPrompt` has told the model to name the
readings and ask which one since the prompt was written — but the answer came back as **prose**,
so the only way to answer it was to retype the whole question. Same for the two caveats that
matter most (the part the documents don't cover, two sources disagreeing): real rules, buried
mid-paragraph.

Now: the readings arrive as a tickable card with the likeliest pre-ticked, a pick composes the
next question, and caveats render as coloured panels. Nothing here changed retrieval.

## Decisions settled — do not relitigate

**The transport is in-band markdown, not a fifth SSE event.** `internal/server/chat.go` fixes
the wire at four frames (`token` · `citations` · `done` · `error`). A `clarify` event would have
cost a Go type, a branch in `chatHandler`, a branch in `lib/chat.js`, a field on the turn shape
and a guide passage about a wire event — for something GFM already gives free. The block is a
`> [!QUESTION]` / `> [!NEXT]` blockquote plus a GFM checklist, which is syntax every model
already writes: the argument `dressTaskLists` makes in its own header, applied to the other
half. **Backend change was prompt-only**, and `promptSig` (`internal/rag/qa.go`) invalidated the
answer cache by construction.

**Checkboxes, not radios.** A radio group would get "exactly one, non-empty" free from
`required`, and that was the first design. Rejected on the ask: two readings of one question are
sometimes both wanted ("how do these differ?"), and that is a question the corpus can answer.
The zero-pick case needed no code — `composeClarify` returns `""` and `ask()` already returns on
a blank question, so an empty submit is a silent no-op.

**`clarify` is derived at render time, never stored on the turn.** It was going to be a field set
in `useConversation`'s `finally`, which forced a `newTurn` field, a `regenerate` reset and a
`session.js` `settle()` entry so a pending card survived a reload. All three disappeared: the
block is still in `turn.a`, which already persists, so `turnClarify(turn)` reads it back for
free. Rule 17 — the stored copy was a second copy of a fact.

**Retrieval untouched.** Query expansion / HyDE was considered and declined: no measurement says
retrieval is missing anything, it costs a provider call on every question plus an env knob
(which rule 15 turns into a documented section), and the actual defect was *guessing intent*.
Asking is cheaper and more correct than guessing better. Revisit only with a measured baseline.

**No test file, deliberately.** `ponytail` asks for one runnable check behind non-trivial logic;
rule 21 says extend the file that owns the rule and verify against the running product, and
rule 21 wins (precedence: a rule outranks a skill). There is also nowhere to put one — `web/ui`
has no JS test runner at all (`package.json` scripts are `dev/build/preview/lint/lint:fix`), and
the ambiguity rule depends on real model output that `internal/aitest`'s fake cannot script.
Verification of record: `make smoke` for the prompt, a real browser for the rendering.

## Landmines found while building this

- **`marked` emits a `space` token between a blockquote and the list under it.** Treating
  "followed by" as `tokens[i + 1]` matched only blocks written with *no* blank line — i.e. the
  cramped ones — and missed every well-formed block. `after()` in `answer.js` skips them; the
  block's tokens are dropped as a *range* so the blank line goes too.
- **`.callout` on a `<blockquote>` inherits the pull-quote recipe.** The library styles
  `blockquote` italic and caps it at a reading measure, and `.callout` undoes neither, so panels
  rendered in italics. `dressAlerts` emits a `<div class="callout …">` — the markup the guide
  pages already use for the same recipe — which also makes the pass idempotent for free.
- **Everything inside `.prose` is capped at `--prose-measure`; a sibling of it is not.** The
  clarify card came out at 1207px against 646px of answer above it, reading as a second column
  rather than the next thing to do. It joins `.q` and `.sources` on that token in `styles.css`
  — a class was added (`.clarify`) purely so the selector could name it.
- **A fresh `clarify` object on every parent render resets the reader's ticks.** `turnClarify`
  is a `computed` in `ChatTurn.vue` for identity, not for cost: without it, somebody else's
  answer arriving below wiped a half-finished pick. Verified in a browser — ticked an option,
  asked an unrelated question, tick survived.
- **`isMiss` is an exact match, and `[!NEXT]` is the rule that would have broken it.** Anything
  appended to the no-answer sentence makes `isMiss` false and caches the gap as an answer. The
  prompt says that sentence stands alone; the coupling is now named in `isMiss`'s own comment, so
  the next trailing block added to the prompt needs the same exception.
- **`pkill -f 'bin/knowledge'` kills the deployed instance.** `/opt/knowledge/bin/knowledge`
  matches that pattern. systemd brought it back in under a second (`NRestarts` went to 11) and
  `/api/health` on :8080 is green, but a local verification server should be stopped by its port
  or its full path, never by that substring.

## State outside git

Nothing. The verification instance ran on :8124 against a scratch DB and is stopped; the
scratch DB is deleted. `web/dist` is rebuilt and **uncommitted** along with the source changes —
rule 14 needs it committed in the same change.

## Live, against the deployed corpus

`[!NEXT]` confirmed in production (`d4d82ae`, "work calendar hoạt động thế nào?"):

```
> [!NEXT] 
- Làm thế nào để thêm hoặc chỉnh sửa ngày làm việc cụ thể?
- Có những quy tắc kinh doanh nào liên quan đến lịch làm việc?
- Ai là những người dùng có thể truy cập vào Work Calendar?
```

Three questions the retrieved sections can answer, none pre-ticked. It also **ignored the syntax
twice** — no label after the marker, and plain `- ` bullets instead of `- [ ]` — and the card
still renders: `marked` types both as `list`, `checked` is `undefined` on a non-task item so
`recommended` is `false`, and `LABEL` supplies the name. Verified against that exact string.

So both fallbacks are load-bearing, not defensive padding.

## Open

- The label fallback is now the **common** path rather than the exception, which was the
  condition set for revisiting it: the prompt line asking the model to name the block is what
  should tighten, not `LABEL`. Same for `- [ ]` — worth one firmer clause, since the checklist is
  what carries `recommended` and a plain bullet silently loses it.
- The `[!QUESTION]` path still has not been seen from a real provider: it needs a corpus that
  genuinely covers one phrase two ways. Rendering is verified from a seeded session in a real
  browser; the *prompt* half is not.
