# 2026-07-30 — ASK BA never fired, and the diagram stopped being fitted

Four defects reported from a phone against the deployed instance, all four measured at 390×844
in the running product (own `pinchtab` instance, own `DB_PATH`, one fixture document, one
provider call — the second ask of the same question was `cached · free`, which is the cache
doing its job).

## ASK BA did nothing, in every build, silently

`ChatTurn.vue` emitted **`askBA`** and every listener up the chain was `@ask-ba`. Those do not
meet: the compiler camelises a kebab listener, so `@ask-ba` binds `onAskBa`, while
`emit("askBA")` resolves `toHandlerKey(event)` then `toHandlerKey(camelize(event))` — both
`onAskBA`. The kebab fallback exists only for `update:*` model listeners. Two capitals in a row
are the one shape that does not round-trip, and nothing says so outside a dev-mode warning:
`grep -o "onAskB[Aa]" web/dist/assets/index-*.js` printed `onAskBa` three times against
`askBA` four times, in the committed bundle.

So the whole QA loop was unreachable from the ASK screen — the one tap that turns a bad answer
into a ticket. The event is `askBa` now, in the three files that name it. **The lesson is a
naming rule, not a fix:** an event name whose kebab-case does not round-trip is a broken
listener, so no emit name gets two adjacent capitals. `diagramDrawn`, `zoomDiagram` and the
rest were fine for that reason and stayed as they were.

Verified end to end: ticket #1 filed, toast, and the turn's badge reading `OPEN #1 — Waiting
for a BA.`

## The in-answer diagram is no longer fitted — the decision inverted

`changelog/`-worthy because it reverses a recorded one. The old rule was *fitted in the answer
(the card carries the shape), read it in `<nes-zoom>`*, and what it shipped was measured here:
a flowchart whose natural size is **938×64** squeezed into 289px draws **20px tall**, six
labels at about 4px. A walkthrough under it then spotlit nodes nobody could see. An overview
that cannot be read is not an overview.

`inline-size: 1000%` on the answer's SVG is the whole change, and it introduces no number about
a diagram: mermaid renders with `useMaxWidth: true`, which is `width="100%"` **plus an inline
`max-width: <natural>px`**, and an inline style outranks the library's `max-inline-size: 100%`
(measured: `max-width` computes to `938.391px`). Ten times the box therefore resolves to
exactly that cap. `.mermaid-view` was already `overflow: auto`, so the overflow is a scroll
inside the card and the page still never scrolls sideways.

What was tried and rejected, so it is not re-tried:

- **`inline-size: auto`** — for an *outermost* `<svg>`, `auto` is defined as `100%`. It measured
  288.891px, i.e. no change at all.
- **A `min-block-size` on the SVG, or `block-size` on it** — the inline `max-width` clamps the
  width back to natural, so `preserveAspectRatio` letterboxes and the drawing stays the size it
  was inside a taller box. A floor on the *frame* alone is the same trap: at `min(34svh, 15rem)`
  the box was 240px with a 20px drawing centred in it, and the citations went off screen.
- **The library's `nes:theme` seam with `useMaxWidth: false`** — it works and it is the
  documented way in, but it costs a listener plus a list of every mermaid diagram type, and it
  throws away the natural width the markup already carries.

The floor stayed, modest: `min(22svh, 9rem)`. Measured 144px against a 100px content box, so a
legible band gains a stage without pushing the sources off a phone screen — the reason the 70vh
maximum has always been there. `dev.html` says all of this in both languages now, and the two
sentences it replaced are in `retiredClaims` (rule 26).

## Two spacing defects, one cause each

- **The document form was a wall.** `.card` is not a flex container and the library spaces only
  *inside* a `.field` (`gap: --sp-2`: label → control → hint), so eight sibling blocks stacked at
  **0px** — a field's hint sat against the next field's label, closer to it than to its own
  input, and read as that label's caption. Same fix `.ticket` and `.drop` already carry: one
  property on the parent, `gap: --sp-5`, the head's non-flex margin zeroed, and `row-gap` on the
  two-up rows whose inherited `--sp-3` made a wrapped pair sit tighter than the fields around
  it. Every seam in the form now measures 24px, inside the pairs included.
- **`EDITING go/basic/go-why.md`** — the eyebrow and the path are the same fact at two sizes,
  and on a 390px sticky head the word took a third of the row from the only thing that has to
  stay readable. It is `edit`/`plus` from the library's icons now, with the word in `title`.

## Left alone deliberately

Two things noticed while measuring, neither in scope, both real:

- **`ImportPanel.vue` still says "Written into `docs/` and indexed immediately."** Nothing writes
  a corpus file (invariant 1) — this is rule 26's failure mode inside the app's own copy rather
  than on a guide page, and `retiredClaims` only reads the published pages.
- **`ingest <absolute-path>` stores absolute document paths.** `fixture/go/basic/go-why.md` when
  the root is relative; the full `/tmp/...` path when it is not. Invariant 5 says an absolute
  path is refused, and `make ingest DOCS=…` passes whatever it is given.
