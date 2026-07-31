# 2026-07-31 — The text came to the column: `--prose-measure` fills the card and caps at 112ch

Reported three times as three symptoms of one fact: an answer's content stopped about halfway
across its own card. The first two reports were fixed by moving *boxes* onto the card's edge (the
walkthrough, then the clarify card). This is the third, and it is the one that needed the measure
to move.

**The first attempt was wrong and is recorded on purpose.** I brought `--col` down from 80rem to
52rem so the card met the text. It measured well (21px of slack at 1600px) and it was the wrong
answer: it squeezes the container on a large screen — the opposite of using it — and it spends the
width a table, a wide diagram and a code block are here for. Reverted the same session, on the
report "why did you squeeze the container instead of filling the content".

## What the measurement said

At a 1130px window the card held **two coherent groups**, not the raggedness the screenshot
suggested:

| block | right edge |
|---|---|
| `.head` · `.prose` · `nes-code` · `.feedback` | 1095 |
| `.callout` · `p` · `ol` · `.sources` | 684 |

So the existing rule held — every drawn box on one edge, text on a measure. The complaint was the
**411px of empty card** beside the measure group (469px at 1600px).

Then the number that decides everything: at 1600px the card's content box is **1248px** and `1ch`
is **8.97px**, so filling it with prose means **139 characters a line**. Past about 100 the eye
loses the start of the next line on the return sweep, which is why the library ships 72.

## What changed

`--prose-measure: min(100%, 112ch)` on `#app`, where the library ships a flat `72ch`.

This is a **retune, not an override** — the library's own comment on the measure says "Retune with
`--prose-measure`", so it does not belong in AGENTS.md's override table.

`100%` is the card's content box, so text *fills the container* at every width narrower than the
cap. Past that the cap holds. One declaration, no media query: a `min()` against `100%` is already
responsive and has no jump at a breakpoint the way a ladder of four values would.

Measured, with the container back at 80rem:

| viewport | card content | text | fill | chars/line |
|---|---|---|---|---|
| 390 | 325 | 325 | **100%** | 83 |
| 768 | 695 | 695 | **100%** | 90 |
| 1130 | 1057 | 1005 | 95% | 112 |
| 1600 | 1207 | 1149 | **95%** | 112 |
| 2560 | 1207 | 1149 | 95% | 112 |

The character count *stops* at 112 while the pixel width keeps growing, because `1ch` is fluid
here — the body font scales with the viewport, so the same 112ch is 1005px at 1130 and 1149px at
1600. Filling more of the card and keeping the line length fixed turn out to be the same
declaration.

## The callout was the last box on the wrong side

A callout inside an answer is a *drawn box*, and it was still capped at the text measure: measured
at 1600px its border stopped 58px short of the card while `nes-code` below it went the full width.
That is the mismatch the screenshot was actually pointing at.

Two declarations, because the box and its text want different widths:

```css
.prose > .callout { max-inline-size: none; }
.prose > .callout > * { max-inline-size: var(--prose-measure); }
```

Without the second line the panel would be a 1200px box of 139-character lines.

After, at 390 / 768 / 1130 / 1600: **`drawnSpread: 0`** — the card's content edge, the callout's
border and the actions row share one edge at every width.

## The 16px this uncovered, and the token that fixed it

While the column was briefly narrow, a pre-existing misalignment became obvious: the prompt was
**16px wider on each side than the card above it**, under a rule whose stated job is that "the
prompt has to look attached to the answers above it".

Root cause: `--col` meant two things. `main` and `.dock` both cap at it, but a card sits inside
`main`'s inline padding while the prompt sits inside the dock's — so the prompt's box was *the
column* and the card's was *the column minus that padding*. Measured at 1130px: prompt 149..981,
card 165..965.

Fixed with a token, not a number: `--gutter-x` on `#app`, used by `main`'s padding, the dock's
padding, and the prompt's cap as `calc(var(--col) - 2 * var(--gutter-x))`. The desktop breakpoint
moves the token instead of two paddings, so the alignment is true by construction — `aligned:
true` at 390, 768, 1130 and 1600. **This one survives the revert**, because it was never about the
column's value.

## `.diagram-zoom` stopped following the column

It was `min(96vw, var(--col))`. It is `min(96vw, 80rem)` now — the same number today, written out
so that the next time the column moves for a *reading* reason this does not follow it. Every pixel
that dialog does not use is a pixel of diagram nobody can see; a text column and a drawing's
window answer different questions. Measured at 1600px: **viewer 1280, card 800** while the column
was still 52rem, which is what made the decoupling visible.

## Two measuring notes, both earned the hard way

- **Compare boxes in the same group.** Two "defects" in this session were box models, not bugs:
  `pre` "missing the card edge by 3px" is `pre` inside `.codeblock`'s border, and `.prose` right vs
  card right differ by the card's own padding. Border box to border box, or same-group only.
- **A headless tab is `visibilityState: "hidden"`, so CSS animations are throttled.** The drawer
  measured entirely off-screen — exactly like the bug an override had just been deleted for — while
  `getAnimations()` reported `drawer-in:running` and nothing moved. Remove the animation before
  measuring a resting position.
