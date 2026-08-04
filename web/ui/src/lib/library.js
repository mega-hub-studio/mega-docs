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

const NONE = { state: 'unavailable', docs: 0, chunks: 0, approved: 0, documents: [] }

/**
 * Fetch the indexed corpus. Resolves to a usable object in every case.
 * @returns {Promise<Corpus>}
 */
export async function loadCorpus() {
  try {
    const res = await fetch('/api/corpus')
    if (!res.ok)
      return NONE
    const c = await res.json()
    // "ready" needs retrievable *chunks*, not just document rows. A failed ingest
    // can leave documents with nothing indexed under them, and calling that ready
    // is how you get a UI that promises answers it can never give.
    return { ...c, state: c.chunks > 0 ? 'ready' : 'empty' }
  }
  catch {
    return NONE
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
  const q = query.trim().toLowerCase()
  const ranked = [...(documents || [])].sort((a, b) => b.chunks - a.chunks)
  // `includes("")` is true for every string, so this early return changes no result — it
  // skips one array and N `docTitle()` calls (measured 54µs at the server's 100-document
  // ceiling) on the empty-query path, which is every render until somebody types.
  if (!q)
    return ranked
  return ranked.filter(d =>
    `${docTitle(d)} ${d.path || ''}`.toLowerCase().includes(q))
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
  return `What does ${docTitle(doc)} cover?`
}

/**
 * The whole document on one line, for the `title` of a row that cannot show it all.
 *
 * A library row shows the name and the path, both ellipsised, because a row whose height
 * depends on its content is a list with nine different geometries. The alias and the
 * description are what the Find field searches — that is their job — so they belong where
 * they can be read on demand rather than in a third and fourth line on every row.
 *
 * @param {{ path?: string, alias?: string, description?: string }} doc
 * @returns {string} path · alias · description, skipping whatever is not set
 */
export function docTip(doc) {
  return [doc.path, doc.alias, doc.description].filter(Boolean).join(' · ')
}

/** A file name is not a sentence: "booking-list_v2.md" → "booking list v2". */
export function docTitle(doc) {
  const name = doc.title || (doc.path || '').split('/').pop() || ''
  return name.replace(/\.[a-z0-9]+$/i, '').replace(/[-_]+/g, ' ').trim()
}

/* Built once at module scope, not per call. `toLocaleDateString` re-resolves the locale
   and rebuilds a formatter every time: measured 78µs a call against 2.0µs through a hoisted
   Intl.DateTimeFormat, 39× cheaper. It matters because the first screen now owns a text
   field, so every keystroke re-evaluates every template expression in the component —
   including the date on each of up to 100 tickets in a *collapsed* disclosure. That was
   5.6ms of avoidable work per keystroke at the server's ticket ceiling. */
const SHORT_DATE = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' })

/** "2026-07-25T09:10:11Z" → "25 Jul" — dense enough for a phone row. */
export function shortDate(iso) {
  const d = new Date((iso || '').replace(' ', 'T') + (iso?.endsWith('Z') ? '' : 'Z'))
  return Number.isNaN(+d) ? '' : SHORT_DATE.format(d)
}

/**
 * "1 doc", "3 docs". Here rather than in the two places that count folders, because
 * "1 docs" is the kind of wrong that makes a screen look unfinished — and it appeared in
 * both the library's headers and the ASK screen's tree, which is a fact wanting one home.
 *
 * @param {number} n how many
 * @param {string} word the singular
 * @returns {string} the number and the word, agreeing
 */
export function plural(n, word) {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

/**
 * What a folder header counts, written the way a person reads it.
 *
 * @param {number} docs how many documents are under the folder
 * @param {number} sections how many retrievable sections they add up to
 * @returns {string} e.g. "3 docs · 12 sections", or "1 doc · 1 section"
 */
export function folderCount(docs, sections) {
  return `${plural(docs, 'doc')} · ${plural(sections, 'section')}`
}
