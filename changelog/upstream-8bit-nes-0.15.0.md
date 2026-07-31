# Upstream requests — 8bit-nes 0.15.0

Three findings from a consumer app, each measured against the pinned bytes of `0.15.0` rather
than its changelog. Same format as the 0.14.0 round, and for the same reason: landing one
upstream **removes** a rule from a consumer instead of adding one.

**All six of the 0.14.0 requests have landed** — `.cite` text-box trim, `.prose a:not(.cite)`,
the conditional `will-change` on `nes-zoom .zoom-stage`, `nes-walkthrough` in the `.prose`
opt-out list, `:is(dialog, .modal) .palette-list`, and `inset-inline-start: auto` on `.drawer`.
Several shipped with the reasoning copied into comments, which is why re-deriving them costs
nothing now. Do not re-report them.

One finding was **withdrawn before sending**, and it is recorded here so the next round does not
resurrect it: `.datalist`'s `@container (max-width:320px)` fallback was reported in 0.13.0 as
never firing. It fires in 0.15.0 — `.card` declares `container-type: inline-size`
(`components.css:257`), so a `.datalist` inside a card has a container. Checked, not assumed.

The `--fill` item below is mostly **this repo's own mistake**, and it is still worth sending: the
library has one dead declaration that is what made the mistake look correct.

---

## PROMPT — copy from here

You maintain **8bit-nes**. A consumer app pinned to **0.15.0** hit three problems, each measured
against `components.css` in the published package with no consumer CSS involved. Line numbers are
that file. Two are defects; one is an accessibility request.

Each item is: **what breaks → the declaration → the measurement → the fix.**

### 1 · `.prose > img` is opted out of the *container*, not just the reading measure — `components.css:1848-1873`

```css
.prose > * { max-inline-size: var(--prose-measure); }
.prose > :is(.table-wrap, table, pre, /* … */ img, svg, video, nes-code, nes-mermaid,
              nes-zoom, nes-walkthrough, /* … */) { max-inline-size: none; }
```

The opt-out list is right in principle and its stated rule — *if the content is the width, it
opts out* — is right too. But every other entry on it **has a way to be too wide safely**: a
`<pre>` and a `.table-wrap` scroll inside themselves, `nes-zoom` pans, `nes-code` scrolls. An
`<img>` has no such escape, so `max-inline-size: none` hands it its **intrinsic** width and it
simply overflows.

Measured with `all.css` alone, no consumer stylesheet, a 1400×300 image as a direct child of
`.prose` inside a `.card`, viewport 390px:

| | |
|---|---|
| `getComputedStyle(img).maxInlineSize` | `none` |
| image width | **1400px** |
| card width | **374px** |
| `documentElement.scrollWidth` / `innerWidth` | **1430 / 390 — the page scrolls sideways** |

Sideways scroll on a phone is the one failure the library's own mobile guidance calls absolute,
and this reaches it from a stylesheet-only page with one image in it.

Two different caps are being conflated. `--prose-measure` is a **reading** limit an image should
escape; `100%` is a **container** limit nothing may. `svg` and `video` are on the same list with
the same exposure.

**Fix:** give the media entries the container cap back rather than removing every cap —

```css
.prose > :is(img, svg, video) { max-inline-size: 100%; block-size: auto; }
```

after the `none` rule, or split the list so media never receives `none`. That keeps the intent
(an image is not held to 72ch) and drops the side effect (an image is not held to anything).

### 2 · `.table td` inherits the browser's `vertical-align: middle` — `components.css:1318`

```css
.table td { padding: var(--sp-3); border-block-end: …; font-variant-numeric: tabular-nums; }
```

No `vertical-align`, so cells are middle-aligned. That is the right default for a `<td>` in a
prose document and the wrong one for a data grid, which is read **across**: the moment one cell
in a row wraps, every short cell in that row floats to the middle of its height and no two
values share a top edge.

It shows up as soon as the table is narrow, which on a phone is always. Measured at 390px, a
four-column table of short Vietnamese phrases: one cell 303px tall beside three one-line cells,
each of those centred somewhere in the middle of 303px. The row reads as four unrelated
fragments rather than one record.

**Fix:** `vertical-align: top` on `.table th, .table td`. It is one declaration, it is what every
data-grid recipe ships, and a consumer cannot add it without re-specifying the selector.

While there, the same table has `th { white-space: nowrap }` and no equivalent floor on `td`, so
headers hold width and body columns collapse to their longest word. Not a bug — but a
`min-inline-size` note in the docs, or a `.table.compact` variant, would save the next consumer
the measurement.

### 3 · `.callout` distinguishes six kinds by hue alone — `components.css:314…`

```css
.callout   { --accent: var(--gold); }
.callout.tip { --accent: var(--good); }   .callout.gotcha { --accent: var(--crit); }
.callout.memo{ --accent: var(--gold); }   .callout.quest  { --accent: var(--purple); }
.callout.info{ --accent: var(--blue); }   .callout.warn   { --accent: var(--warn); }
```

The recipe is a border, a tint and `& b { color: var(--accent) }` — nothing else. So *which kind
of callout this is* is carried entirely by colour, and `memo` (`--gold #fbd000`) against `warn`
(`--warn #ff9e2c`) is two warm yellows; `warn` against `gotcha` (`--crit #f23c4e`) is the
warning/danger pair that matters most. This is WCAG 2.2 **1.4.1 Use of Color** — colour is the
only visual means of conveying the distinction.

It bites hardest where callouts are generated rather than hand-written. A consumer rendering
GitHub alert syntax (`> [!WARNING]`, `> [!CAUTION]`, …) into `.callout` has to strip the marker,
because leaving it renders `[!WARNING]` as literal text — and then the panel's kind exists
nowhere on screen but the border.

**Fix (any one):** a per-kind `::before` glyph the consumer can turn off; an `.callout > .kind`
label slot in the recipe; or documented `nes-icon` names per kind so a consumer emits the right
one. The consumer currently prepends its own emoji (⚠️ 📝 💡 ❗ 🛑) into the panel's first
paragraph — a workaround the library could make unnecessary in three lines.

### 4 · Not a bug, one dead line: `.pbar { --fill: 0% }` — `components.css:616`

```css
@property --fill { syntax: "<percentage>"; inherits: false; initial-value: 0%; }  /* :7 */
.pbar     { --fill: 0%; … }                                                       /* :616 */
.pbar > i { inline-size: var(--fill); … }                                         /* :625 */
```

With `inherits: false`, the value at `:616` **cannot be read by anything** — `.pbar` itself never
uses `var(--fill)`, and the child resolves the registration's own `initial-value` instead. The
declaration is unreachable.

Your docs are correct and this consumer did not follow them: the published example is
`<span class="pbar"><i style="--fill:64%"></i></span>`, and the app set `--fill` on the container
because that is what `:616` implies. The result was a determinate bar stuck empty at every
count, with the right number in the sentence beside it and no error anywhere. Measured at the
same 66.66%: **0px with the value on `.pbar`, 1031.89px with it on the `<i>`.**

**Fix:** delete `:616` — the registration already supplies `0%`, so it is pure signal loss. Or,
if the container form should work, make it work with `inherits: true`. Either one closes the
gap; leaving a dead declaration that reads as the API is the only bad option.

---

### One thing that would have caught two of these

**Render the component gallery once at 390px and assert `documentElement.scrollWidth ===
innerWidth`.** #1 fails it outright, and #2 is visible in the same screenshot. Both are invisible
at desktop width, which is where a component gallery is usually looked at.

## PROMPT — copy to here
