/* ══ upload.js — importing documents into the corpus ══════════════════════════
   Hides: multipart assembly, the password header, and the one response shape the
   app must not treat as a plain failure — a 400 whose body still lists which file
   was rejected and why.

     const { uploaded, failed, chunks } = await send(fileList);

   Same password as a confirm, because both change what every reader is told. The
   filter runs here too, so dropping a folder of PDFs costs one message instead of
   one upload.
   ═══════════════════════════════════════════════════════════════════════════ */

import { pass, setPass, WrongPass } from './qa.js'

/** What the file picker offers, and what a drop is filtered against. */
export const ACCEPT = '.md,.markdown,.txt'

/**
 * The kinds the form offers before the corpus has taught it any.
 *
 * `kind` is a filter in the library and a column a reader scans, so its value only earns its
 * keep when everyone spells it the same way. Deriving the list from what is already indexed —
 * which is what the form did — cannot start: the first document is filed under whatever its
 * author typed, the second under a near-miss of that, and `spec` / `Spec` / `specification`
 * become three kinds that mean one thing. Six words are enough to make the common case a pick
 * rather than a guess, and the list is a suggestion: a datalist, never a closed set, so a team
 * with its own vocabulary keeps it and it shows up here beside these.
 */
export const KINDS = ['spec', 'guide', 'decision', 'runbook', 'api', 'qa']

const EXTS = ['.md', '.markdown', '.txt']

/**
 * The folders already in the corpus, deepest paths collapsed to each level, so the
 * picker suggests the structure that exists instead of inviting a fourth spelling
 * of "engineering".
 * @param {{path: string}[]} documents from /api/corpus
 */
export function folders(documents = []) {
  const seen = new Set()
  for (const d of documents) {
    const parts = String(d.path || '').split('/')
    parts.pop() // the file name
    for (let i = 1; i <= parts.length; i++) seen.add(parts.slice(0, i).join('/'))
  }
  return [...seen].sort()
}

/**
 * A file name read back as a title: `refund-policy.md` → `Refund policy`.
 *
 * Only the first letter is raised. Title Case On Every Word would rewrite `ERR_PAY_402` and
 * `api` into something nobody can search for, and an identifier is the one thing this app
 * never translates.
 *
 * @param {string} name the file name, extension and all
 * @returns {string} a title worth offering, or "" when the name says nothing
 */
export function titleFrom(name) {
  const stem = String(name || '').replace(/\.(?:md|markdown|txt)$/i, '').replaceAll(/[-_]+/g, ' ').trim()
  return stem ? stem[0].toUpperCase() + stem.slice(1) : ''
}

/** Split a drop into what can be sent and what cannot, so the UI can say both. */
export function sort(files) {
  const ok = []
  const rejected = []
  for (const f of [...files]) {
    (EXTS.some(e => f.name.toLowerCase().endsWith(e)) ? ok : rejected).push(f)
  }
  return { ok, rejected }
}

/**
 * Does this password open the gate? Asked before a BA is told they are unlocked,
 * because the alternative is what used to happen: the password is stored without
 * being checked, the import card appears, and the first upload turns a wrong
 * password into a card that silently disappears.
 *
 * The probe is a request that changes nothing — no files, so the gate answers
 * first and the handler then rejects it as empty. 401/403 is a real no; anything
 * else means the gate opened.
 * @returns {Promise<boolean>}
 */
export async function verify(candidate) {
  let res
  try {
    res = await fetch('/api/documents', {
      method: 'POST',
      headers: { 'X-BA-Pass': candidate },
      body: new FormData(),
    })
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  return res.status !== 401 && res.status !== 403
}

/**
 * Import documents, one request per file.
 *
 * One request each, not one for the batch: it is what makes the progress bar
 * *true*. A single POST can only be animated by guessing, and a bar that invents
 * its own position is worse than none — it says "nearly done" while the last file
 * has not started. It also means a file's outcome appears as soon as it lands,
 * instead of every file waiting for the slowest.
 *
 * @param {File[]} files
 * @param {string} dir
 * @param {(done: number, total: number) => void} [onProgress] after each file
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 * @returns {Promise<{uploaded: {path: string, chunks: number}[], failed: {name: string, error: string}[], chunks: number}>}
 */
export async function send(files, dir = '', onProgress = () => {}) {
  const out = { uploaded: [], failed: [], chunks: 0 }
  let done = 0
  onProgress(0, files.length)
  for (const f of files) {
    const one = await sendOne(f, dir)
    out.uploaded.push(...one.uploaded)
    out.failed.push(...one.failed)
    out.chunks += one.chunks || 0
    onProgress(++done, files.length)
  }
  return out
}

/**
 * Take one document out of the corpus.
 *
 * The server moves the file into `.trash/` inside the corpus rather than unlinking it, so a
 * mis-click is recoverable with one `mv` — which matters more than usual here, because
 * nothing backs the corpus up automatically any more.
 *
 * Each segment is encoded separately: the route is `{path...}`, so `/` has to survive as a
 * separator while a space or a `#` in a file name must not.
 *
 * @param {string} path the document's identity, as /api/corpus reports it
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 * @returns {Promise<{path: string, trash: string}>} where it went; `trash` is empty when the
 *   file was already gone and only the index row was cleaned
 */
export async function remove(path) {
  let res
  try {
    res = await fetch(docURL(path), { method: 'DELETE', headers: { 'X-BA-Pass': pass() } })
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  if (res.status === 401 || res.status === 403) {
    setPass('')
    throw new WrongPass((await res.text()).trim() || 'Not allowed')
  }
  if (!res.ok) {
    // 400 here is "not a usable path" or "not in the corpus" — a sentence worth showing,
    // not a status code to translate.
    throw new Error((await res.text()).trim() || `Server error ${res.status}`)
  }
  return res.json()
}

async function sendOne(file, dir) {
  const body = new FormData()
  if (dir.trim())
    body.append('dir', dir.trim())
  // webkitRelativePath is set when a *folder* was picked, and it carries the
  // structure the person already built on their own disk — keep it rather than
  // flattening a tree they organised.
  body.append('files', file, file.webkitRelativePath || file.name)

  let res
  try {
    res = await fetch('/api/documents', { method: 'POST', headers: { 'X-BA-Pass': pass() }, body })
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  if (res.status === 401 || res.status === 403) {
    setPass('')
    throw new WrongPass((await res.text()).trim() || 'Not allowed')
  }
  // 400 with a JSON body means "nothing usable", and it still names the file.
  // Throwing that away would leave the user with "Server error 400" for a problem
  // whose description is one line long.
  if (res.headers.get('Content-Type')?.includes('application/json')) {
    return res.json()
  }
  throw new Error((await res.text()).trim() || `Server error ${res.status}`)
}

/**
 * The route for one document. Each segment is encoded separately: the route is `{path...}`,
 * so `/` has to survive as a separator while a space or a `#` in a name must not.
 *
 * @param {string} path the document's identity
 * @returns {string} the URL for GET · PUT · DELETE on it
 */
function docURL(path) {
  return `/api/documents/${String(path).split('/').map(encodeURIComponent).join('/')}`
}

/**
 * Read one document back, attributes and body — what an edit starts from.
 *
 * Ungated, like every other read: this is the same text an answer quotes with a citation
 * pointing at it.
 *
 * @param {string} path the document's identity
 * @returns {Promise<{path: string, title: string, alias: string, kind: string,
 *   description: string, body: string}>} the document as stored
 */
export async function read(path) {
  let res
  try {
    res = await fetch(docURL(path))
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  if (res.status === 404)
    throw new Error('That document is not in the knowledge base any more')
  if (!res.ok)
    throw new Error((await res.text()).trim() || `Server error ${res.status}`)
  return res.json()
}

/**
 * Write one document: its text, its attributes, and where it lives.
 *
 * One call for "new" and for "edit", because the server has one route for both — PUT means
 * "this document, at this path", and a rename is that same sentence with a different path in
 * `to`. Saving is what re-indexes it, so the answer a reader gets next is the text saved here.
 *
 * @param {string} path the document being written; for a new one, where it should land
 * @param {{to?: string, body: string, title?: string, alias?: string, kind?: string,
 *   description?: string}} doc `to` moves it — leave it out to keep the path
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 * @returns {Promise<{path: string, chunks: number}>} where it ended up, and how much of it
 *   reached the index
 */
export async function write(path, doc) {
  let res
  try {
    res = await fetch(docURL(path), {
      method: 'PUT',
      headers: { 'X-BA-Pass': pass(), 'Content-Type': 'application/json' },
      body: JSON.stringify(doc),
    })
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  if (res.status === 401 || res.status === 403) {
    setPass('')
    throw new WrongPass((await res.text()).trim() || 'Not allowed')
  }
  if (!res.ok)
    throw new Error((await res.text()).trim() || `Server error ${res.status}`)
  return res.json()
}
