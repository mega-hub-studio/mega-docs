/* ══ diagram.js — mermaid, but only when there is a diagram ════════════════════
   Hides: that the renderer is 3.4 MB, that it is fetched on demand, and that it
   might never arrive.

     if (hasDiagram(text)) await ready();   // resolves false if it cannot load

   The design system ships <nes-mermaid> and themes it, but deliberately does not
   bundle mermaid itself — "bring your own or lazy-load it". Lazy is the only honest
   option here: the renderer is fourteen times the size of the entire rest of the
   app, and most answers are prose. So it is fetched the first time an answer
   actually contains a diagram, and never on first paint.

   Vite gives it its own chunk: the `import("mermaid")` below is dynamic, so nothing
   about the renderer is in the entry bundle. Do not import it statically anywhere —
   that quietly moves 3.4 MB into first paint, and only the chunk sizes would say so.
   ═══════════════════════════════════════════════════════════════════════════ */

/* The diagram kinds mermaid understands, as they appear on a diagram's first line.
   The list is here rather than inline because two callers must agree: the one that
   decides whether to fetch the renderer, and the one that decides which block to
   replace. If they disagree, the answer either loads 3.4 MB for nothing or shows a
   fence it could have drawn. */
const KIND
  = /^\s*(?:%%\{[^}]*\}%%\s*)?(?:graph|flowchart|sequenceDiagram|classDiagram|stateDiagram(?:-v2)?|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|quadrantChart|requirementDiagram|C4Context|sankey-beta|xychart-beta|block-beta)\b/i

/**
 * Is this code block a diagram?
 *
 * By its content, not by its fence label — because the label is the model's to write
 * and it does not reliably write it. Reported from the deployed app: an answer whose
 * fence was bare and whose first line was `graph TD;` rendered as source code. The
 * prompt asks for a language tag, and a prompt cannot enforce an invariant the code
 * depends on (the same lesson as the NoAnswer sentinel).
 *
 * @param {string} source the block's own text
 */
export function isDiagram(source) {
  return KIND.test(source || '')
}

/**
 * Does this answer contain a diagram? Cheap enough to call per render.
 *
 * Fence-aware on purpose: testing every line would match prose that happens to start
 * with "graph" and fetch the renderer for an answer with nothing to draw.
 */
export function hasDiagram(markdown) {
  let inFence = false
  for (const line of String(markdown || '').split('\n')) {
    if (/^\s*(?:```|~~~)/.test(line)) {
      if (!inFence && /^\s*(?:```|~~~)\s*mermaid\b/i.test(line))
        return true
      inFence = !inFence
      continue
    }
    if (inFence && isDiagram(line))
      return true
  }
  return false
}

/* ── reading a diagram on a phone ──────────────────────────────────────────────
   Two things a reader wants, and they conflict at 390px: the *shape* of the whole
   diagram, and the *labels*. Mermaid scales to fit, which gives the first and loses
   the second — measured at 1083px squeezed into 289, so 16px text drew at about 4.

   So the answer keeps the drawing at its natural size in a frame that scrolls, and reading
   it is three verbs that all work *in place*: drag it (`pannable`), zoom it (`controls`), or
   open it full screen. The last one was the only one for a while, and it was reported twice —
   once as "cannot drag", once as "why do I have to go full screen to zoom". A preview you can
   only look at is a picture of a diagram.

   What still belongs to <nes-zoom> and is not reimplemented here: pinch, wheel, reset, and the
   panning of the *copy* it holds. In place, pan is the frame's own `scrollLeft` and zoom is one
   CSS `zoom` factor on the SVG — no transform, no second panner, no pinned width. */

/**
 * Give a freshly drawn diagram its three verbs: drag, zoom, full screen.
 *
 * Called from the library's own `nes:render` event. Nothing happens for a diagram that
 * fell back to source text — there is nothing to drag, zoom or open.
 *
 * @param {Element} host the <nes-mermaid> that just drew
 */
export function onRender(host) {
  if (!host?.querySelector?.('svg') || host.dataset.zoomable)
    return
  host.dataset.zoomable = '1'
  host.append(controls())
  pannable(host.querySelector('.mermaid-view'))
}

/* Zoom is one number on the drawing, because the frame around it is already a scroll
   container: CSS `zoom` scales the SVG's *used size*, so the scroll area grows with it and
   `pannable` below reaches every part of it. Measured on a 759×510 drawing at `zoom: 2`:
   1519×1020 with scrollWidth 1535. `transform: scale(2)` looks identical and is wrong — the
   box paints twice as big while layout does not, so scrollWidth stayed 1155 and the overflow
   could not be panned to.

   It also keeps the app out of the business the retired claim named: nothing here pins a
   width. A factor is what a zoom is, and mermaid's own natural size stays the 1.0. */
const STEP = 1.25
const RANGE = [0.5, 4]

/* ── the bar under a diagram ────────────────────────────────────────────────────
   Three buttons, and only two of them have a listener.

   `⤢` needs none: `useDiagrams.open` keys on `e.target.closest("nes-mermaid")`, so a click
   anywhere inside the host already opens the viewer — a real <button> inside it therefore
   *is* the keyboard door, and it replaced the `role="button"` + `tabindex` this function used
   to put on the host. That swap is the point: a host carrying `role="button"` with two
   focusable children inside it is a widget with interactive descendants, which is the one
   thing that role may not have. One `<button>` beats a div pretending to be one.

   `−` and `+` stop propagating, or zooming in place would also open full screen. */
function controls() {
  const bar = document.createElement('span')
  bar.className = 'zoom-hint'
  bar.append(
    step('−', 'Zoom out', 1 / STEP),
    step('+', 'Zoom in', STEP),
    button('⤢ FULL SCREEN', 'Open this diagram full screen'),
  )
  return bar
}

function step(glyph, label, factor) {
  const b = button(glyph, label)
  b.addEventListener('click', (e) => {
    e.stopPropagation() // …or the viewer opens on top of the zoom that was asked for
    const svg = b.closest('nes-mermaid')?.querySelector('svg')
    if (!svg)
      return
    const now = Number(svg.style.zoom) || 1
    svg.style.zoom = Math.min(RANGE[1], Math.max(RANGE[0], now * factor))
  })
  return b
}

function button(text, label) {
  const b = document.createElement('button')
  b.type = 'button'
  b.className = 'btn ghost xs'
  b.textContent = text
  b.setAttribute('aria-label', label)
  return b
}

/** Swallows the click that ends a real drag, so panning never also opens the viewer. */
const swallow = e => e.stopPropagation()

/**
 * Drag a diagram that is wider than its frame — with a mouse, which is the case the platform
 * leaves out.
 *
 * The frame is already a scroll container, so this is `scrollLeft`/`scrollTop` and nothing
 * here remembers a position: a touch drags it natively (hence the `pointerType` guard, or a
 * finger would move it twice), a trackpad has two fingers, and a mouse has an 8px scrollbar at
 * the bottom of a 550px card — which reads as "the diagram is cut off", not as "drag it".
 * Reported exactly that way.
 *
 * Only the drag: the zoom beside it is `controls` above, and pinch, wheel and reset stay the
 * viewer's — reimplementing those here would be a second panner to keep in agreement with the
 * first.
 *
 * @param {Element|null} view the library's `.mermaid-view`, or null when the render failed
 */
function pannable(view) {
  if (!view || view.dataset.pan)
    return
  view.dataset.pan = '1'
  let from = null

  view.addEventListener('pointerdown', (e) => {
    if (e.pointerType !== 'mouse' || e.button !== 0)
      return
    from = { x: e.clientX, y: e.clientY, left: view.scrollLeft, top: view.scrollTop, moved: false }
    // Keeps the drag alive past the frame's edge. Not every browser will hand it over, and a
    // drag that stops at the border is still better than no drag.
    try {
      view.setPointerCapture(e.pointerId)
    }
    catch {}
  })

  view.addEventListener('pointermove', (e) => {
    if (!from)
      return
    const dx = e.clientX - from.x
    const dy = e.clientY - from.y
    // A tap with a tremor in it is still a tap: under the threshold nothing scrolls and the
    // click goes on to open the viewer.
    if (!from.moved && Math.hypot(dx, dy) < 4)
      return
    from.moved = true
    view.classList.add('is-panning')
    view.scrollLeft = from.left - dx
    view.scrollTop = from.top - dy
  })

  const end = () => {
    view.classList.remove('is-panning')
    // A drag must not also open the viewer. One capture-phase listener, once, so the tap on a
    // still pointer keeps working — the alternative is a flag two files apart deciding what a
    // click meant.
    if (from?.moved)
      view.addEventListener('click', swallow, { capture: true, once: true })
    from = null
  }
  view.addEventListener('pointerup', end)
  view.addEventListener('pointercancel', end)
}

/**
 * A walkthrough moved: bring the node it just lit inside the diagram's own scroller.
 *
 * The library highlights but does not scroll — `_reapplyFocus` toggles the classes and
 * stops there — so on a phone this is the difference between a walkthrough and a stepper
 * attached to an unchanging picture. Measured at 390×844: the box is capped at 48svh
 * (405px) and a seven-node graph draws 554px, so the last two steps lit a node below the
 * fold and the reader saw the sentence change and the diagram not.
 *
 * `nearest` scrolls the diagram box and nothing else. The page must not move on NEXT: the
 * answer, its citations and the prompt are all in the same column, and a jump there costs
 * the reader their place to save a scroll they did not ask for.
 *
 * @param {Event} e the library's bubbling `nes:step`, fired after the highlight lands
 */
export function onStep(e) {
  const id = e.target?.getAttribute?.('for')
  const lit = id && document.getElementById(id)?.querySelector('.nes-focus')
  lit?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
}

/**
 * Put a copy of a diagram into the viewer, and hand the panner back to 1:1.
 *
 * A copy, not the original: moving the node out of the answer would leave a hole in it
 * and lose the drawing when the dialog closes. Nothing sizes it — `.zoom-stage` fits the
 * SVG to the frame and the reader scales it from there.
 *
 * @param {Element} into the frame inside <nes-zoom>'s stage
 * @param {Element} host the <nes-mermaid> that was tapped
 * @returns {boolean} false when there was no drawing to show
 */
/**
 * Show an answer's image in the same viewer a diagram uses.
 *
 * A separate function rather than a branch inside `zoomInto` because the two share nothing but
 * the destination: an SVG needs its id-scoped styles rewritten (`reid`) or the clone steals the
 * original's rules, and an image needs none of that — it is one node with a src.
 *
 * @param {Element} into the viewer's stage
 * @param {HTMLImageElement} img the image that was tapped
 * @returns {boolean} whether there was anything to show
 */
export function zoomImage(into, img) {
  if (!into || !img)
    return false
  const copy = img.cloneNode(true)
  copy.removeAttribute('tabindex') // inside the viewer it is the subject, not a control
  copy.removeAttribute('loading') // it is the only thing on screen; do not defer it
  into.replaceChildren(copy)
  into.closest('nes-zoom')?.reset?.()
  return true
}

export function zoomInto(into, host) {
  const svg = host?.querySelector?.('svg')
  if (!into || !svg)
    return false
  const copy = svg.cloneNode(true)
  reid(copy, svg.id)
  into.replaceChildren(copy)
  // The pan/scale lives on the component, not on what it holds, so a second diagram would
  // otherwise open wherever the first one was dragged to.
  into.closest('nes-zoom')?.reset?.()
  return true
}

/**
 * Give a cloned diagram its own id, and move its stylesheet with it.
 *
 * Mermaid scopes every rule it emits to the diagram's id — `#nes-mmd-1-1 .node rect`
 * — inside a `<style>` element within the SVG. So a copy needs *a* unique id and its
 * own rules rewritten to match: leaving the original id duplicates it in the document,
 * and dropping it silently un-themes the copy, which renders as black boxes on a black
 * background. That was the first version of this, and it looked like a broken renderer.
 */
function reid(copy, oldId) {
  if (!oldId)
    return
  const fresh = `${oldId}-zoom`
  copy.id = fresh
  for (const style of copy.querySelectorAll('style')) {
    style.textContent = style.textContent.split(`#${oldId}`).join(`#${fresh}`)
  }
}

let loading = null

/**
 * Make sure mermaid is available.
 * @returns {Promise<boolean>} false when it could not be loaded — the caller then
 *   leaves the fenced code block alone, which is a worse diagram but a real answer.
 */
export function ready() {
  if (window.mermaid)
    return Promise.resolve(true)
  // One in-flight load, however many diagrams arrive at once.
  loading ??= load().catch(() => {
    loading = null // a later answer may succeed where this one failed
    return false
  })
  return loading
}

async function load() {
  const m = await import('mermaid')
  // <nes-mermaid> looks at globalThis.mermaid first and only falls back to fetching a
  // URL, so handing it the module here is what keeps the renderer a bundled chunk
  // instead of a second network request. The element still applies its own theme to an
  // instance it did not create — including the label size, which it reads from --mmd-fs
  // (styles.css says why this app sets it).
  globalThis.mermaid = m.default ?? m
  return true
}
