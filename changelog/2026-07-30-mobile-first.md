# 2026-07-30 — mobile-first becomes rule 28, and the overflow it caught immediately

Asked to make the app mobile-first and minimal, and to add a critical rule for it. The rule was
the smaller half: `styles.css` has said *"base styles are the phone — never the reverse"* since it
was written, and its comments carry two dozen measurements taken at 320 · 360 · 390. **The
practice was already there; nothing in the rules held it.** So a change could be reasoned about at
laptop width and measured afterwards — which is exactly what happened one turn earlier, in this
repo, by me.

## What measuring the app at 390×844 actually found

Almost nothing, which is the useful result — and one thing that mattered.

| probe | result |
|---|---|
| sideways scroll | `documentElement.scrollWidth === innerWidth === 390` — none |
| anything past the viewport with no scroller of its own | **empty**. The Go block reaches 440px and scrolls inside `.codeblock pre`, which is correct |
| left edges of the answer card's five children | **one value, 34px** — `head`, `prose`, the clarify card, `sources`, `feedback` all agree |
| left edges of every `.prose` block | **one value, 34px** |
| clipped text | none |

So the column was already a column. No spacing was changed, no rule was added to `styles.css` for
its own sake, and the "extremely minimal" part of the request needed no redesign — it needed the
one defect below removed and the discipline written down so the next change cannot skip it.

## The defect, and it was mine from the turn before

Unwrapping a lone-image paragraph — so the library's `img` opt-out from `--prose-measure` would
apply — handed a 1400px screenshot `max-inline-size: none`. Measured: it rendered **1404px inside
a 1207px card**. On a 390px phone that is the page scrolling sideways, the one thing rule 28 calls
absolute.

The cause is a conflation, and it is worth stating because 8bit-nes states the principle correctly
and the principle is still not enough: the opt-out list exists because *"if the content is the
width, it opts out"*, and every other entry on it — `<pre>`, `.table-wrap`, `<nes-zoom>`,
`.codeblock` — scrolls or pans **inside itself**. An `<img>` has no such escape. So
`max-inline-size: none` removed two caps where only one should have gone:

| cap | who it belongs to | should an image escape it? |
|---|---|---|
| `--prose-measure` (72ch) | reading comfort | **yes** — a screenshot is not prose |
| `100%` | the container | **never** |

`.prose > img { max-inline-size: 100% }` takes the second one back. Verified
**container-relative**, so the number holds at any width: natural 1400 · container 1207 · rendered
**1207** · still escaping the 646px measure. Both goals at once, which is what the two-cap split
buys.

## Rule 28, and what enforces it honestly

- **The guide is measured.** `make check-ui` runs every page at **390 · 1440 · 1920** (390 with
  `mobile: true` and dpr 3, Playwright's iPhone 14 spelled out) and fails a section wider than its
  parent. That is a real enforcer and it already existed.
- **The app is measured by nothing**, and rule 27 already admits the same for seams. A rig for one
  screen is what rule 21 refuses. What replaces it is two probes that cost nothing, both listed in
  the rule: `scrollWidth === innerWidth`, and *every child of the answer card reports one `left`*.
  Five blocks sharing an edge read as a column; one block 2px off reads as a mistake nobody can
  name — which is the whole of the "minimal" request, expressed as something measurable.

## Landmines

**PinchTab cannot emulate `pointer: coarse`.** `set media pointer coarse` reports "applied" while
`matchMedia("(pointer: coarse)")` stays false — CDP emulates a fixed set of features and `pointer`
is not among them. So **touch-target heights are unchecked here** and rest on 8bit-nes' own release
testing. `scripts/check-docs-ui.mjs:36` already said this; re-deriving it cost a few minutes and is
recorded here so the next attempt does not.

**`pinchtab set viewport` reports success while `innerWidth` does not move**, when the instance is
shared with an MCP session — it acts on *the instance's current tab*, which is the flakiness
CLAUDE.md already warns about for `check-ui`. Six retries all printed `390x844` and all measured
1438. The way out was not to fight it: the question — does a wide image fit its container — is
**container-relative**, so it is answerable at any viewport, and that measurement is the one in the
table above. When a phone-width number is genuinely required, use the rig (`scripts/guide-rig.sh`),
which owns its own instance.

## Not changed, deliberately

No spacing, no new layout rule, no breakpoint. The measurements said the column was already
aligned, and inventing a redesign to look responsive to the request would have been the opposite of
what was asked.
