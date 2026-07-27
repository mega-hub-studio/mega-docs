# 2026-07-27 — A diagram in an answer is a diagram, not source code

Reported from the deployed app with a screenshot: asked *"calendar sơ đồ như nào"*, the
answer arrived as a fenced block of mermaid text. The renderer had shipped the same day
and was working — it just never fired.

## Why it didn't fire

Both halves of the feature keyed on the **fence label**: `hasDiagram()` matched
```` ```mermaid ```` to decide whether to fetch the renderer, and `asDiagrams()` matched
`class="language-mermaid"` to decide which block to replace. The model wrote a bare
```` ``` ```` with `graph TD;` on the first line.

The prompt does ask for a language tag. That is the same mistake as the `NoAnswer`
sentinel, in a new place: **a prompt cannot enforce an invariant the code depends on.**

So detection is now by content — the diagram kinds mermaid understands, matched against
a block's first line — and the kind list lives in one place used by *both* callers.
Split between them, they would eventually disagree: fetch 3.4 MB and draw nothing, or
find a block and have no renderer.

A restored thread now triggers the load too. It only happened when an answer *arrived*,
so a reload — which a phone does constantly — left an existing diagram as source code
forever.

## Reading it on a phone

The first fix made it worse in a way worth recording. Pinning the drawing to its natural
width made the labels legible, and left a 390px screen showing about a third of a
1083px flowchart: readable, and no longer a diagram you can take in.

Both are wanted and they conflict at that width, so: **fitted in the answer, natural
size on demand.** The card keeps mermaid's fit-to-width — the shape, whole — plus a
`⤢ TAP TO ZOOM` hint, `role="button"` and a tab stop. A tap opens a copy in the
library's `<dialog class="modal">`, sized from its `viewBox`, scrolling in both axes
inside `.mermaid-view` so it is styled exactly like the original.

Three things that cost a measurement each:

- **CSS cannot size the copy.** An SVG with a `viewBox` has no intrinsic width, so
  `width: auto` resolves against the container and mermaid's `useMaxWidth` shrinks it
  again. Measured: 1083px squeezed into 289, drawing 16px labels at about 4. The number
  exists only in the `viewBox`, so `diagram.fit`/`zoomInto` read it there.
- **The copy needs its own id.** Mermaid scopes every rule it emits to the diagram's id
  inside a `<style>` in the SVG. Removing the id un-themes the copy — black boxes on a
  black background, indistinguishable from a broken renderer — and keeping it duplicates
  an id in the document. `reid()` renames both the attribute and the rules.
- **The event to hang this on already existed.** `<nes-mermaid>` dispatches a bubbling
  `nes:render`, so one listener on the answer catches every diagram inside `v-html`,
  which Vue never sees as components.

## Verified

In a real browser at 390px and 1440px, against the vendored renderer (no network):

- the reported answer — bare fence, `graph TD;` — renders as 8 nodes, no source left
- **the live streamed path too**, with a provider returning a bare fence: source shows
  while the 3.4 MB renderer loads, then the drawing replaces it
- the card shows the whole diagram (no inner scroll), the zoom shows it at 1083px with
  13.5px labels, panning in both axes
- node fill in the copy matches the original exactly (`rgb(28, 28, 86)`), one id in the
  document, Escape closes, the viewer empties, and the card keeps its diagram
- no console errors, and the page never scrolls sideways at either width

One thing to remember about my own verification: the first run reported the zoomed SVG
as 9px wide. That was the test selecting `dialog svg`, which matched the close icon.
The selector was wrong, not the code.
