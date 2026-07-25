/* ══ session.js — the conversation survives a reload ══════════════════════════
   Hides: localStorage, quota failures, schema drift, and the fact that a turn
   caught mid-stream must not come back looking like it's still generating.

     const turns = load();     // [] on a first visit or anything unreadable
     save(turns);              // debounced, never throws

   Phones reload pages constantly — backgrounded tabs get evicted, a share sheet
   round-trips, the browser updates. Losing the thread every time is the single
   most annoying thing a mobile chat UI can do.
   ═══════════════════════════════════════════════════════════════════════════ */

const KEY = "ke.session.v1"; // bump the suffix if the turn shape changes
const MAX_TURNS = 30; // a phone doesn't need more, and quota is finite
const SAVE_DELAY = 400; // coalesce a burst of streamed tokens into one write

/** @returns {Array<object>} the stored turns, or [] */
export function load() {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const turns = JSON.parse(raw);
    if (!Array.isArray(turns)) return [];
    return turns.filter(valid).map(settle);
  } catch {
    return []; // unreadable or from an older shape — start clean, don't crash
  }
}

let timer = null;
let pending = null;

/** Persist the conversation. Debounced; failures are ignored on purpose. */
export function save(turns) {
  // Snapshot now, write later: turns are Vue proxies and will keep changing
  // while the debounce waits.
  pending = turns.slice(-MAX_TURNS).map(settle);
  clearTimeout(timer);
  timer = setTimeout(flush, SAVE_DELAY);
}

export function clear() {
  clearTimeout(timer);
  pending = null;
  try {
    localStorage.removeItem(KEY);
  } catch {
    /* nothing to do */
  }
}

function flush() {
  clearTimeout(timer);
  if (!pending) return;
  try {
    localStorage.setItem(KEY, JSON.stringify(pending));
  } catch {
    // Private mode, or quota exhausted. Persistence is a convenience: the
    // conversation on screen is unaffected, so there is nothing to report.
  }
}

/* A phone rarely closes a tab — it backgrounds it, and iOS may then evict the
   page without ever running another timer. pagehide/visibilitychange are the last
   guaranteed moments to write, so the debounce never costs a lost answer. */
addEventListener("pagehide", flush);
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") flush();
});

function valid(t) {
  return t && typeof t.q === "string" && t.q !== "";
}

/* A turn that was streaming when the page went away is finished as far as the
   restored session is concerned — otherwise it comes back with a spinner that
   never stops and a Stop button wired to nothing. */
function settle(t) {
  return {
    id: t.id,
    q: t.q,
    a: t.a ?? "",
    citations: Array.isArray(t.citations) ? t.citations : [],
    error: t.error ?? "",
    ms: t.ms ?? 0,
    streaming: false,
  };
}
