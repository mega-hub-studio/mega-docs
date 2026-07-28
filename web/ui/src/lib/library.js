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
    // "ready" needs retrievable *chunks*, not just document rows. A failed ingest
    // can leave documents with nothing indexed under them, and calling that ready
    // is how you get a UI that promises answers it can never give.
    return { ...c, state: c.chunks > 0 ? "ready" : "empty" };
  } catch {
    return NONE;
  }
}

/**
 * The documents worth tapping first, ranked and optionally narrowed by a typed query.
 *
 * Ranked by retrievable sections, because that is what makes a document *able* to
 * answer — a row with 2 sections and a row with 300 are not equally worth the first tap,
 * and the count is shown next to each so the order never has to be taken on trust.
 *
 * The query matches title *and* path: a folder name ("booking/") is how anyone with a
 * structured corpus actually looks, and it is invisible in the title alone.
 *
 * @param {Doc[]} documents
 * @param {string} query
 * @returns {Doc[]}
 */
export function rankDocs(documents, query) {
  const q = query.trim().toLowerCase();
  const ranked = [...(documents || [])].sort((a, b) => b.chunks - a.chunks);
  if (!q) return ranked;
  return ranked.filter((d) =>
    `${docTitle(d)} ${d.path || ""}`.toLowerCase().includes(q));
}

/**
 * The question a document row sends.
 *
 * It was three hardcoded questions about this engine once ("How do I ingest a PDF?") —
 * which, over a corpus of booking specifications, advertises the wrong subject and
 * returns "not in the documents" for all three. "What does it cover?" is instead the
 * question that proves the whole loop — retrieval, grounding, citation — in one tap.
 *
 * Built here, in one place, because the row no longer *shows* this sentence: it shows
 * the document, and the sentence is what the tap means. Two spellings of it would be
 * two different cache keys for what a user experiences as one question.
 *
 * @param {Doc} doc
 * @returns {string}
 */
export function coverQuestion(doc) {
  return `What does ${docTitle(doc)} cover?`;
}

/** A file name is not a sentence: "booking-list_v2.md" → "booking list v2". */
export function docTitle(doc) {
  const name = doc.title || (doc.path || "").split("/").pop() || "";
  return name.replace(/\.[a-z0-9]+$/i, "").replace(/[-_]+/g, " ").trim();
}

/** "2026-07-25T09:10:11Z" → "25 Jul" — dense enough for a phone row. */
export function shortDate(iso) {
  const d = new Date((iso || "").replace(" ", "T") + (iso?.endsWith("Z") ? "" : "Z"));
  return Number.isNaN(+d)
    ? ""
    : d.toLocaleDateString(undefined, { day: "numeric", month: "short" });
}
