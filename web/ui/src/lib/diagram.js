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
const KIND =
  /^\s*(?:%%\{[^}]*\}%%\s*)?(?:graph|flowchart|sequenceDiagram|classDiagram|stateDiagram(?:-v2)?|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|quadrantChart|requirementDiagram|C4Context|sankey-beta|xychart-beta|block-beta)\b/i;

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
  return KIND.test(source || "");
}

/**
 * Does this answer contain a diagram? Cheap enough to call per render.
 *
 * Fence-aware on purpose: testing every line would match prose that happens to start
 * with "graph" and fetch the renderer for an answer with nothing to draw.
 */
export function hasDiagram(markdown) {
  let inFence = false;
  for (const line of String(markdown || "").split("\n")) {
    if (/^\s*(?:```|~~~)/.test(line)) {
      if (!inFence && /^\s*(?:```|~~~)\s*mermaid\b/i.test(line)) return true;
      inFence = !inFence;
      continue;
    }
    if (inFence && isDiagram(line)) return true;
  }
  return false;
}

/* ── reading a diagram on a phone ──────────────────────────────────────────────
   Two things a reader wants, and they conflict at 390px: the *shape* of the whole
   diagram, and the *labels*. Mermaid scales to fit, which gives the first and loses
   the second — measured at 1083px squeezed into 289, so 16px text drew at about 4.

   So: fitted in the answer, natural size on demand. The card keeps the overview, and
   one tap opens the same drawing full-screen where it can be panned and pinched. */

/**
 * Mark a freshly drawn diagram as openable, and say so.
 *
 * Called from the library's own `nes:render` event. Nothing happens for a diagram that
 * fell back to source text — there is no drawing to open.
 *
 * @param {Element} host the <nes-mermaid> that just drew
 */
export function onRender(host) {
  if (!host?.querySelector?.("svg") || host.dataset.zoomable) return;
  host.dataset.zoomable = "1";
  host.setAttribute("role", "button");
  host.setAttribute("tabindex", "0");
  host.setAttribute("aria-label", "Open this diagram full screen");
  const hint = document.createElement("span");
  hint.className = "zoom-hint";
  hint.textContent = "⤢ TAP TO ZOOM";
  host.append(hint);
}

/**
 * Put a copy of a diagram into the viewer at its natural size.
 *
 * A copy, not the original: moving the node out of the answer would leave a hole in it
 * and lose the drawing when the dialog closes. The width comes from the viewBox because
 * that is the only place it exists — an SVG with a viewBox has no intrinsic width, so
 * CSS `width: auto` just resolves against the container again.
 *
 * @param {Element} into an empty container inside the dialog
 * @param {Element} host the <nes-mermaid> that was tapped
 * @returns {boolean} false when there was no drawing to show
 */
export function zoomInto(into, host) {
  const svg = host?.querySelector?.("svg");
  if (!into || !svg) return false;
  const copy = svg.cloneNode(true);
  const box = svg.getAttribute("viewBox")?.trim().split(/[\s,]+/);
  const width = box?.length >= 4 ? Math.round(Number.parseFloat(box[2])) : 0;
  if (width) {
    copy.style.inlineSize = `${width}px`;
    copy.style.maxInlineSize = "none";
  }
  reid(copy, svg.id);
  into.replaceChildren(copy);
  return true;
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
  if (!oldId) return;
  const fresh = `${oldId}-zoom`;
  copy.id = fresh;
  for (const style of copy.querySelectorAll("style")) {
    style.textContent = style.textContent.split(`#${oldId}`).join(`#${fresh}`);
  }
}

let loading = null;

/**
 * Make sure mermaid is available.
 * @returns {Promise<boolean>} false when it could not be loaded — the caller then
 *   leaves the fenced code block alone, which is a worse diagram but a real answer.
 */
export function ready() {
  if (window.mermaid) return Promise.resolve(true);
  // One in-flight load, however many diagrams arrive at once.
  loading ??= load().catch(() => {
    loading = null; // a later answer may succeed where this one failed
    return false;
  });
  return loading;
}

async function load() {
  const m = await import("mermaid");
  // <nes-mermaid> looks at globalThis.mermaid first and only falls back to fetching a
  // URL, so handing it the module here is what keeps the renderer a bundled chunk
  // instead of a second network request. The element still applies its own theme to an
  // instance it did not create.
  globalThis.mermaid = m.default ?? m;
  return true;
}
