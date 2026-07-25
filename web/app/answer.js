/* ══ answer.js — markdown → safe, cited HTML ══════════════════════════════════
   Hides: marked configuration, DOMPurify, and the "[n]" → <a class="cite"> pass.

     answerHtml(markdown, { count: 2, srcId: (n) => `s7-${n}` })

   `marked` and `DOMPurify` are globals here on purpose — index.html loads them as
   classic scripts so they cost no module graph. This is the only file that knows.
   ═══════════════════════════════════════════════════════════════════════════ */

let configured = false;

/**
 * Render one answer.
 * @param {string} markdown  raw model output
 * @param {{ count?: number, srcId?: (n: number) => string }} sources
 *   count — how many citations exist; markers above it are left as plain text.
 *   srcId — element id for source n. Omit either and no linking happens (the
 *   right behaviour mid-stream, when the citation list hasn't arrived yet).
 * @returns {string} sanitized HTML
 */
export function answerHtml(markdown, { count = 0, srcId } = {}) {
  if (!configured) {
    marked.setOptions({ breaks: true });
    configured = true;
  }
  const html = DOMPurify.sanitize(marked.parse(markdown || ""));
  return count > 0 && srcId ? linkCites(html, count, srcId) : html;
}

/* Runs on the sanitized DOM, never on the markdown: injecting anchors before
   marked would make a "[1]" inside a code fence render as literal HTML. Text
   inside code/pre — or an existing link — is left exactly as written. */
function linkCites(html, count, srcId) {
  const tpl = document.createElement("template");
  tpl.innerHTML = html;

  const walk = document.createTreeWalker(tpl.content, NodeFilter.SHOW_TEXT);
  const hits = [];
  while (walk.nextNode()) {
    const n = walk.currentNode;
    if (n.parentElement?.closest("code, pre, a")) continue;
    if (/\[\d+\]/.test(n.nodeValue)) hits.push(n);
  }

  for (const node of hits) node.replaceWith(split(node.nodeValue, count, srcId));
  return tpl.innerHTML;
}

function split(text, count, srcId) {
  const frag = document.createDocumentFragment();
  let last = 0;
  for (const m of text.matchAll(/\[(\d+)\]/g)) {
    const n = Number(m[1]);
    if (n < 1 || n > count) continue; // "[0]" or "[99]" points at nothing
    frag.append(text.slice(last, m.index), cite(n, srcId(n)));
    last = m.index + m[0].length;
  }
  frag.append(text.slice(last));
  return frag;
}

function cite(n, href) {
  const a = document.createElement("a");
  a.className = "cite"; // 8bit-nes recipe: superscript marker, jumps to .source
  a.href = `#${href}`;
  a.setAttribute("aria-label", `Source ${n}`);
  a.textContent = String(n);
  return a;
}

/** "docs/architecture/retrieval.md" → "retrieval.md" */
export function fileName(path) {
  return (path || "").split("/").pop();
}
