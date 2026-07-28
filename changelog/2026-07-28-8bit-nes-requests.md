# Requests to 8bit-nes — the spec to take upstream

> **All six landed in 8bit-nes 0.8.0**, as specified. This repo is on that pin, and the
> local workarounds are deleted: the `initialize` wrapper in `diagram.js` (now `--mmd-fs`),
> `main.ba .datalist { align-items: baseline }`, and the `.prose` override — which takes
> `AGENTS.md`'s override count from two back to one. `.palette-list` is the only override
> left, and 0.8.0 does not fix it; that request predates this file and is still open.
>
> Nothing below is outstanding. It is kept as the reasoning behind the release, and as the
> shape to reuse: read the pinned package, name the construct, measure the symptom, say what
> the consumer deletes when it lands.

Pinned version measured against: **8bit-nes@0.7.3** (`web/vendor.sha384`).
Upstream: <https://github.com/TuTranMVP/8bit-components> — the same owner, so these are
changes to make, not tickets to hope for.

This file is two things at once, deliberately: the record of what this app asked for and
why, and a **prompt you can paste into 8bit-components** and work from. Everything below
was found by reading `all.min.css` and `elements.js` of the pinned package while fixing a
real symptom here; each item names the construct, the symptom, and the one thing this
repository deletes when it lands.

Already filed in [`2026-07-28-audit-and-cleanup.md`](2026-07-28-audit-and-cleanup.md) —
do not re-file: an in-page variant of `.palette`, a mono/tabular default for
`.result-hint`, `.callout.crit` as an alias for `gotcha`.

---

## Prompt

> You are working in **8bit-components** (published as `8bit-nes`). Six changes are
> specified below, found by an integrator on `0.7.3`. Four are bugs in the sense that the
> library already does the right thing somewhere else and not here — fix those first
> (2, 3, 4, 6). Two are capability gaps (1, 5).
>
> House rules to keep: zero runtime dependencies; every visual value comes from a token;
> a component owns its own internals; the docs site, `llms.txt`, `llms-full.txt` and
> `components.json` are generated, so a new token or attribute is documented where those
> are generated from, not by hand.
>
> For each item: make the change, then prove it with the smallest thing that fails without
> it. Do not add a test file per item if the suite already has a home for it.

---

## 1 · `mermaidTheme()` has no font size

**Where** `elements.js` → `export function mermaidTheme(root)`

**Now** The theme is fully token-driven except for one value. It reads `--font-body`,
`--ink`, `--line-hi`, `--panel-2`, `--slot`, `--primary`, `--bg` off the live root and maps
them into `themeVariables`. It sets `fontFamily`. It never sets `fontSize`, so mermaid's own
default applies — `fontSize: 16` in mermaid 11.16.0, which lands in the rendered SVG as
`#nes-mmd-1-1{font-family:"NES Sans",…;font-size:16px;fill:…}`.

**Why it matters** 16px is the one number that decides how much of a diagram is legible at
once. `flowchart.useMaxWidth: true` fits the drawing to its container, so a larger font
makes a larger natural drawing, which is then scaled down harder: the labels do not get
bigger, the *diagram* gets smaller. Measured in this app: a 1083px-wide flowchart in a
545px column drew its 16px labels at roughly 4px, and a reader saw four nodes of a
fourteen-node flow without scrolling. Every other lever a consumer has — column width,
`max-block-size`, a zoom viewer — treats the symptom.

**Want** Map it like the rest:

```js
fontSize: v("--mmd-fs", v("--fs-body", "16px")),
```

A dedicated token because a diagram label is not body copy — it should be able to go
smaller than the reading size without dragging prose down with it — falling back to
`--fs-body` so a consumer who sets nothing still gets something coherent with the page.

**Acceptance** With `--mmd-fs: 12px` on `:root`, the rendered SVG carries `font-size:12px`
and the same source produces a smaller `viewBox` than at the default.

**Downstream today** Worked around in `web/ui/src/lib/diagram.js`: before handing the
instance over, `lib.initialize` is wrapped so whatever config the element passes gets
`fontSize` and `themeVariables.fontSize` merged on top. Measured in a browser against a
six-node `graph TD` — emitted `font-size` 16px → 12px, `viewBox` height 723.84 → 617.63, so
the boxes shrink rather than the text shrinking inside boxes of the same size. Note that
only `themeVariables.fontSize` moves the layout; the top-level `fontSize` alone changed
nothing, which is worth knowing when mapping the token.

**Deletes here** that wrapper.

---

## 2 · A brought-your-own mermaid can be themed exactly once, by the library only

**Where** `elements.js` → `NesMermaid.render()`

**Now**

```js
if (globalThis.mermaid && !NesMermaid._themed) {
  lib.initialize(mermaidTheme());
  NesMermaid._themed = true;
}
```

`_themed` is a private static on a class the package does not export. A consumer that brings
its own mermaid — the documented path, and the one that keeps the 3.4 MB renderer out of the
entry bundle — has no supported way to amend that config: initialise before the element and
`initialize()` resets over it; initialise after and there is no hook saying when "after" is.

**Why it matters** There *is* a way through, and it is the wrong shape: wrap
`lib.initialize` on the instance before assigning `globalThis.mermaid`, so the element's own
call carries the extra config. That is what this app does for item 1 and it is measured
working — but it is monkey-patching a method the element is about to call, it depends on the
element calling `initialize` exactly once with the whole config, and nothing in the package
promises either. The element is right to theme an instance it did not create; what traps the
integrator is that the only seam is one nobody designed.

**Want** One of, cheapest first:

- honour `themeVariables`/config overrides from an attribute or a static
  (`NesMermaid.config = {...}`) merged on top of `mermaidTheme()`; or
- dispatch `nes:theme` (cancelable, `detail.config`) before `initialize`, so a consumer can
  amend the object; or
- export the guard as documented API (`NesMermaid.retheme()`).

**Acceptance** A BYO consumer can change one mermaid config value without importing
anything private and without racing the element's first render.

---

## 3 · `.datalist` asks for baseline alignment in a way that cannot work

**Where** `all.min.css` → `.datalist`

**Now**

```css
.datalist{display:grid;grid-template-columns:auto 1fr;gap:var(--sp-2) var(--sp-4);
  font-size:var(--fs-body);
  >dt{…;font-size:var(--fs-label);…;align-self:baseline}
  >dd{margin:0;…}}
```

`align-self: baseline` is on `dt` alone. A baseline-aligned grid item needs another
baseline-aligned item **in the same row** to share a baseline with; a group of one falls back
to `start`. So both boxes sit on the row's top edge while their text does not: `dt` is
`--fs-label` (9px) and `dd` inherits `--fs-body` (13.5px), and their first baselines land a
few pixels apart. The row whose value is an inline `<code>` is worst — its border and padding
grow the line box above the baseline and push the value's text down again.

**Why it matters** The declaration says the intent was baseline all along. And the library
already does it correctly one recipe over: `.source{display:flex;align-items:baseline;…}` is
the same shape — a small mono label beside body text.

**Want** `align-items: baseline` on `.datalist`. The `dt` declaration then agrees with it
instead of degenerating, and `dd` joins the group.

**Acceptance** In a `.datalist` whose `dd` contains an inline `<code>`, the `dt` text and the
`dd` text share a baseline.

**Deletes here** `main.ba .datalist { align-items: baseline; }` in `web/ui/src/styles.css`.

---

## 4 · `<nes-zoom>`'s internals are global class names

**Where** `all.min.css` → `.zoom-view`, `.zoom-stage`, `.zoom-bar`; `elements.js` → `NesZoom`

**Now** The element builds `.zoom-view` / `.zoom-stage` / `.zoom-bar` itself, but the rules
are top-level — only `nes-zoom{position:relative}` is scoped to the element. `.zoom-view`
carries `overflow:hidden`, `cursor:grab` and `touch-action:none`, all of which are correct
*because* the element pans with a transform on `.zoom-stage`.

**Why it matters** Put that class on anything else and you get the appearance of a panner
with none of it. This app did exactly that — a diagram viewer built as
`<div class="mermaid-view zoom-view">` — and the result was a box that showed a grab cursor,
could not be dragged, and, because `.zoom-view`'s `overflow:hidden` is declared after
`.mermaid-view`'s `overflow:auto` at equal specificity, **had no scroll container either**.
A tall diagram was simply clipped. Nothing in the library or the console said why. The repo
already knows this failure mode from `.row` being both a generic row and a tree row.

**Want** Scope the internals to their owner — `nes-zoom .zoom-view { … }` and the same for
`.zoom-stage` / `.zoom-bar` — so the class is inert outside the element it belongs to.

**Acceptance** `<div class="zoom-view">` outside a `<nes-zoom>` receives none of those
declarations.

---

## 5 · `<nes-zoom>` cannot pinch

**Where** `elements.js` → `NesZoom.connectedCallback`

**Now** `wheel` zooms; `pointerdown`/`pointermove` pan; `+ − 0` and three buttons are the
rest. `.zoom-view` sets `touch-action: none`, which is required for the drag and also removes
the browser's own pinch. Nothing implements one in its place.

**Why it matters** On a phone — the case the component exists for, "wrap a big AI-generated
diagram to make it explorable" — the reader can pan with a finger but can only scale by
hunting for the `+` button. Pinch is the gesture people try first.

**Want** Two-pointer pinch from the pointer events already being tracked: keep the active
pointers in a `Map`, and while two are down scale by the ratio of the current distance to the
distance at gesture start, anchored on the midpoint. No dependency, no new listener types.

**Acceptance** Two-finger pinch inside `.zoom-view` scales the stage; one finger still pans;
the mouse path is unchanged.

---

## 6 · `.prose` caps the container, not the text

**Where** `all.min.css` → `.prose`

**Now** `.prose{…;max-inline-size:72ch;…}`.

**Why it matters** 72ch is a reading measure, and it is right — for text. On the container it
also caps every child that is *not* text. In this app a UI-specification table inside a
`.prose` answer was crushed to one word per column ("Panel" breaking to "Pane" / "l") with a
third of the card empty beside it, and a diagram drew its labels smaller to fit a width it did
not need to fit. A paragraph rewraps when it runs out of room; a table, a `<pre>` and an SVG
cannot.

**Want** Move the cap onto the children that want it, keeping the container free:

```css
.prose { /* no max-inline-size */ }
.prose > * { max-inline-size: 72ch; }
.prose > :is(.table-wrap, pre, hr, nes-mermaid, img) { max-inline-size: none; }
```

Measured by default is the safe direction — a construct nobody has considered yet reads
correctly rather than sprawling — and only the things whose content *is* width opt out.

**Acceptance** A `.table-wrap` inside a 1200px `.prose` is 1200px wide; a `<p>` beside it is
still 72ch.

**Deletes here** the `.prose` override in `web/ui/src/styles.css`, which
[`AGENTS.md`](../AGENTS.md) currently has to name as one of this app's two permanent
overrides of the design system.

---

## Not filed, on purpose

- `h1,h2,h3{…text-wrap:balance}` is **correct**. It reads wrong in this app only because
  `.q` is an `<h2>` carrying body copy — our choice for the document outline, our override.
- `@media(pointer:coarse){.btn{min-block-size:2.75rem}}` already does the right thing on the
  right axis; a local 44px rule here was deleted rather than reported.
- `.mermaid-view`'s frame inside another frame: the library already drops it under
  `nes-tabs.lens`, so the pattern exists. Not worth an API.

## Acceptance for the whole set

`8bit-nes` publishes a version where items 3, 4 and 6 are fixed; this repo bumps
`web/vendor.sha384`, re-runs `make check-ui`, and deletes two local rules and one of the two
entries in `AGENTS.md`'s override list. Items 1 and 2 together delete the `initialize`
wrapper in `diagram.js` — the workaround works, but it is a private seam this app should not
be standing on. Item 5 is the only one with no downstream answer at all.
