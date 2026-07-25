/* ══ library.js — what the engine knows ═══════════════════════════════════════
   Hides: the /api/corpus call and its failure modes.

     const corpus = await loadCorpus();   // never throws
     corpus.state                         // "ready" | "empty" | "unavailable"

   `state` exists so the UI has one thing to branch on. Without it every caller
   would re-derive "is this an empty index or a dead server?" from docs === 0 plus
   a try/catch, and get it subtly wrong.
   ═══════════════════════════════════════════════════════════════════════════ */

/** @typedef {{ path: string, title: string, chunks: number, approved: number, updated_at: string }} Doc */
/** @typedef {{ state: "ready"|"empty"|"unavailable", docs: number, chunks: number, approved: number, documents: Doc[] }} Corpus */

const NONE = { state: "unavailable", docs: 0, chunks: 0, approved: 0, documents: [] };

/**
 * Fetch the indexed corpus. Resolves to a usable object in every case.
 * @returns {Promise<Corpus>}
 */
export async function loadCorpus() {
  try {
    const res = await fetch("/api/corpus");
    if (!res.ok) return NONE;
    const c = await res.json();
    return { ...c, state: c.docs > 0 ? "ready" : "empty" };
  } catch {
    return NONE;
  }
}

/** "2026-07-25T09:10:11Z" → "25 Jul" — dense enough for a phone row. */
export function shortDate(iso) {
  const d = new Date((iso || "").replace(" ", "T") + (iso?.endsWith("Z") ? "" : "Z"));
  return Number.isNaN(+d)
    ? ""
    : d.toLocaleDateString(undefined, { day: "numeric", month: "short" });
}
