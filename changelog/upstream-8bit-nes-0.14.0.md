# Upstream requests — 8bit-nes 0.14.0

Six findings from a consumer app, each measured against the pinned bytes of `0.14.0` rather than
its changelog. Every one is currently patched locally in that app (`web/ui/src/styles.css`), and
each local patch names the release condition that deletes it — so landing one upstream removes a
rule from a consumer instead of adding one.

Paste the block below into the library repo. It is written to be actionable without this repo:
selector, the exact declaration at fault, why it is wrong, what was measured, and the proposed
fix. Nothing about mega-docs is load-bearing in it.

A seventh has already landed and is recorded here so it is not re-reported: **`nes-walkthrough`
is blocked** (`components.css:460`) — the consumer's local `display: block` was deleted on
2026-07-30 after being redundant for at least one release.

---

## PROMPT — copy from here

You maintain **8bit-nes**. A consumer app pinned to **0.14.0** patched six recipes locally and
measured each one against `components.css` in the published package. Line numbers are that file.
Fix them at the source so the consumer can delete its overrides.

Each item is: **what breaks → the declaration → the measurement → the fix.**

### 1 · `.cite` centres its line box, not the digit's ink — `components.css:3091`

```css
.cite { display: inline-flex; align-items: center; justify-content: center;
        font-size: .7em; line-height: var(--lh-heading); /* … */ }
```

Flexbox centres the text run's **em box**, and for a digit that box is not symmetric around the
ink. Measured with the font's own metrics at the chip's `.7em` (9.8px NES Mono): em box ascent 9 /
descent 3, a digit's ink ascent 7 / descent 0 — so the centred box carries 2px of unused space
above the glyph and 3px below, and the ink lands **0.5px high** (one device pixel at 2× DPR) in a
15.7px chip. Slack above/below: `3.86 / 4.84`.

**Fix:** `text-box-trim: trim-both; text-box-edge: cap alphabetic;` on `.cite`. It trims the
half-leading to cap height and the alphabetic baseline, so `align-items: center` then centres the
ink exactly at every size and no padding constant is needed. Degrades to today's behaviour where
unsupported. Verified in the consumer with a `0.1em` padding shim: slack becomes `4.83 / 4.84`.

### 2 · `.prose a` outscores `.cite`, so every citation renders as a prose link — `components.css:1863`

Both live in the components layer; `.prose a` is (0,1,1) against `.cite`'s (0,1,0), so a
`<a class="cite">` inside `.prose` takes the prose-link colour and underline. Measured on a built
bundle: `rgb(86,211,100)` with a 2px underline **through a digit already sitting in a cyan chip** —
a second affordance struck through the one thing on the line that is not an ordinary link.

**Fix:** scope the prose-link rule away from the chip — `.prose a:not(.cite)`, or raise `.cite`'s
own colour/decoration to match its specificity. Either removes the consumer's
`.prose a.cite, .prose a.cite:hover { color: var(--cyan); text-decoration: none }`.

### 3 · `nes-zoom .zoom-stage` hints a transform whose content it cannot know — `components.css:4028`

```css
nes-zoom .zoom-stage { inline-size: 100%; transform-origin: center center; will-change: transform; }
```

`will-change: transform` promotes the stage to a composited layer, and the layer is **rasterised
once at scale 1** — so every later `scale(s)` from `_apply()` stretches that bitmap. Correct for
the `<img>` the same rule also holds; wrong for an inline SVG, which would otherwise be redrawn
from its vectors at the new size.

Reported by users as *"zoom mode is blurry"*. Captured twice at `zoomTo(3)`, same drawing, same
scale: soft labels with a halo on every box edge, then razor sharp with `will-change: auto`.

**Fix:** do not hint the transform unconditionally. Either drop it, or set it only when the stage
holds a raster — e.g. `nes-zoom .zoom-stage:has(> img) { will-change: transform }`. A vector wants
to be re-rasterised per scale; a raster wants the layer.

### 4 · `nes-walkthrough` is missing from `.prose`'s width opt-out list — `components.css:1836`

```css
.prose > * { max-inline-size: var(--prose-measure); }
.prose > :is(.table-wrap, table, pre, /* … */ nes-code, nes-mermaid, nes-graph, nes-zoom) {
  max-inline-size: none;
}
```

`nes-mermaid` opts out, `nes-walkthrough` does not — but the two are a pair: the stepper annotates
the drawing above it. Measured at a 1220px window: a **1185px** drawing annotated by a **646px**
stepper, two right edges 500px apart inside one card.

**Fix:** add `nes-walkthrough` to that `:is()` list. Its width is its diagram's, not the prose's.

### 5 · `.palette-list` sizes itself for a modal it is not always in

```css
.palette-list { max-block-size: min(50vh, 340px); overflow-y: auto; }
```

The recipe assumes the `.palette` lives in a modal, as its own docs describe. Used in a page it
becomes a **nested** scroller inside the page's own — measured on a phone: four of seven rows
unreachable, because the outer scroll ended before the inner one began.

**Fix:** ship an in-page variant that does not cap itself (`.palette.inline`, or move the cap onto
`.modal .palette-list`). The consumer currently un-caps it with `max-block-size: none`.

### 6 · `.drawer[open]` is over-constrained on a `<dialog>`

The recipe anchors with `inset-inline-end: 0` and leaves the start inset alone — correct on a plain
box. On a `<dialog>` the UA sheet sets `inset-inline: 0`, so together with the recipe's `margin: 0`
the box is over-constrained and **the start inset wins**. Measured: the panel opened flush against
the **left** edge, and `.drawer.start` rendered identically — i.e. the modifier had no effect.

**Fix:** qualify the recipe's own rule — set `inset-inline-start: auto` alongside
`inset-inline-end: 0` (and the mirror for `.drawer.start`), so a `<dialog>` and a plain box anchor
the same way.

---

### Two things that would have caught most of these

- **A `<dialog>` case in the drawer's own docs.** #6 is invisible on a plain box and certain on a
  dialog, and the docs only show the box.
- **One SVG in the zoom example.** #3 is invisible with the `<img>` the example uses and certain
  with vector content, which is what a diagram viewer holds.

## PROMPT — copy to here
