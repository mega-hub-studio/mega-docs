/* ══ diagram.js — mermaid, but only when there is a diagram ════════════════════
   Hides: that the renderer is 3.4 MB, that it is fetched on demand, and that it
   might never arrive.

     if (hasDiagram(text)) await ready();   // resolves false if it cannot load

   The design system ships <nes-mermaid> and themes it, but deliberately does not
   bundle mermaid itself — "bring your own or lazy-load it". Lazy is the only honest
   option here: the renderer is fourteen times the size of the entire rest of the
   app, and most answers are prose. So it is fetched the first time an answer
   actually contains a diagram, and never on first paint.

   The URL and its sha384 come from the same <script type="application/json"> the
   page renders out of web/vendor.sha384, so this file spells out no version and no
   digest — a bump stays one line in one manifest.
   ═══════════════════════════════════════════════════════════════════════════ */

/** Does this answer contain a mermaid block? Cheap enough to call per render. */
export function hasDiagram(markdown) {
  return /```\s*mermaid/i.test(markdown || "");
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

function load() {
  const cfg = JSON.parse(document.getElementById("mermaid-src")?.textContent || "null");
  if (!cfg?.src) return Promise.reject(new Error("no mermaid source configured"));

  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = cfg.src;
    // Same guarantee as every other asset on the page: the browser refuses a byte
    // that does not match. A failed check rejects, and the code block stays.
    if (cfg.integrity) {
      s.integrity = cfg.integrity;
      s.crossOrigin = "anonymous";
    }
    s.onload = () => resolve(!!window.mermaid);
    s.onerror = () => reject(new Error("mermaid failed to load"));
    document.head.append(s);
  });
}
