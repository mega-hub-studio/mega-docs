# 2026-07-30 — The viewer was scaling a picture, the preview could not be dragged, and one card had three right edges

Three reported from a browser, three measured before and after. Two are upstream requests
(AGENTS.md's table, now six); one is ours.

## Zoom mode was blurry, and `align-items: center` was not the reason

`nes-zoom` scales with `transform: scale(s)` on `.zoom-stage`, and the recipe hints
`will-change: transform` on it. That promotes the stage to a composited layer, the layer is
**rasterised once at scale 1**, and every later scale stretches that bitmap. Correct for the
`<img>` the same rule holds; exactly wrong for an SVG, which is the only thing in this app that
is vector and the only thing this dialog ever holds.

Captured at `zoomTo(3)` twice, same diagram, same scale: soft labels with a halo on every box
edge, then razor sharp with `will-change: auto`. One declaration, scoped to `.diagram-zoom` so
an `<img>` in some other `<nes-zoom>` keeps the recipe's hint.

**Upstream request #5:** do not hint `will-change: transform` on a stage whose content the
component cannot know. A raster wants the layer; a vector wants to be re-rasterised per scale.

## The preview could not be dragged — with a mouse

The in-answer frame is a scroll container, so touch pans it natively and a trackpad has two
fingers. A mouse has the scrollbar: 8px at the bottom of a 550px card, which reads as *the
diagram is cut off*, not as *drag it*. Reported in exactly those words.

`lib/diagram.js` gains `pannable()`, wired into the `onRender` hook that already marks a drawn
diagram zoomable — so no new event plumbing, and no second copy of the panner:

- `scrollLeft`/`scrollTop` on the frame. Nothing here remembers a position; a scroller already
  does.
- `pointerType !== 'mouse'` returns early, or a finger would move the diagram twice.
- 4px threshold: a tap with a tremor in it is still a tap, and the click goes on to open the
  viewer.
- A real drag swallows exactly one click, capture phase, `once` — otherwise every pan would
  also open the viewer. The alternative was a flag two files apart deciding what a click meant.

Scale stays the viewer's. `<nes-zoom>` owns pinch, wheel and the buttons, and a second panner
is a second thing to keep in agreement with the first.

Verified with real `PointerEvent`s: `scrollLeft` 0 → 120 → 240, `is-panning` on during and off
after, then `openedByDrag: false` and `openedByTap: true` in the same run.

## One card, three right edges

Measured at a 1220px window, inside one answer card:

| block | right edge |
|---|---|
| head · diagram frame · actions row | 1185 |
| **walkthrough** | **684** |
| **clarify card** | **684** |

Two different causes behind one symptom, which is why the fix is in two places:

1. **`nes-walkthrough` is not on the library's opt-out list.** `.prose > *` carries the text
   measure and the constructs whose content *is* width opt out — `nes-mermaid` is on that list,
   the stepper is not, because upstream has no reason to expect the two paired. A 1185px drawing
   annotated by a 646px box. **Upstream request #6.**

   Found while compacting these requests for the library: the `display: block` this rule also
   carried had **already landed upstream** (0.14.0 blocks the element at `components.css:460`),
   so the app had been restating a fact the library makes for at least one release. Deleted —
   from `styles.css` and from AGENTS.md's row. That is the whole argument for AGENTS.md's
   "re-measure every override on a bump rather than trusting its own comment": the comment said
   "reported upstream" and was right, and nobody came back for the second half of the sentence.
2. **`.clarify` was capped by a rule of ours**, and that rule's own comment says why: a 1207px
   form under 646px of prose "read as a different column". True then, and the answer it sits
   under is a *diagram* now — so the capped form was the one box in five with its own edge. It
   is uncapped, and the old reasoning stays in the comment as what changed.

After: **spread 0px** across head, diagram, walkthrough, clarify and the actions row.

What deliberately did **not** move, because measuring said it was invisible: `.sources`, `.q`
and the BA screen's paragraphs keep `--prose-measure`. A citation row's content measures 109px
inside a 646px box — widening it changes no pixel a reader can see, and none of the three draws
a right edge. That is the line the rule stops at now: **every box that is drawn shares one edge;
only text keeps a measure.** Prose wider is one token on `.a` (`--prose-measure`) if it is ever
wanted, not a rewrite.
