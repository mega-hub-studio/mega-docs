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
  const url = `/api/documents/${String(path).split('/').map(encodeURIComponent).join('/')}`
  let res
  try {
    res = await fetch(url, { method: 'DELETE', headers: { 'X-BA-Pass': pass() } })
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
