/* ══ qa.js — the BA ⇄ DEV loop and the history panel ══════════════════════════
   Hides: the /api/tickets shape, the BA password (where it's kept, when it's
   sent), and the difference between "this instance is read-only" and "your
   password is wrong" — two 4xx codes the UI has to react to differently.

     const { tickets, open, … } = await queue();
     await file(question, miss);              // a DEV reports a gap — no password
     await act(id, "confirm", { answer });    // a BA publishes it — password
     const rows = await history();            // answers that now cost nothing

   The password lives in sessionStorage, not localStorage: a shared laptop should
   forget it when the tab closes, and a BA session is one sitting.
   ═══════════════════════════════════════════════════════════════════════════ */

const KEY = 'ke.ba.pass'

/**
 * The four ticket states, and how each one should read on screen. One table, so
 *  a new state can never arrive with no label — see internal/db/tickets.go.
 */
export const STATUS = {
  open: { label: 'OPEN', badge: 'warn', accent: 'gold', hint: 'Waiting for a BA.' },
  answered: { label: 'DRAFT', badge: 'todo', accent: 'blue', hint: 'Answered, not yet published.' },
  confirmed: { label: 'IN KNOWLEDGE', badge: 'clear', accent: 'good', hint: 'Indexed — retrieved with a citation.' },
  rejected: { label: 'DISMISSED', badge: 'crit', accent: 'crit', hint: 'Not a documentation gap.' },
}

/**
 * Where a confirm will publish, so a BA can read it before pressing the button.
 *
 * A *preview*, not the decision: `rag.qaPath` in Go is the authority and does this plus every
 * structural check a path gets (no `..`, no hidden segment, MaxDepth). The three rules a BA has
 * to see are the three this repeats — `qa/` is not typed, `.md` is added, and an empty name
 * falls back to the ticket's id — because a name whose result is invisible until after it is
 * published is a name nobody dares change.
 * @param {string} name what the BA typed, or ""
 * @param {number} id the ticket, for the fallback
 * @returns {string} the path a citation will print
 */
export function docPath(name, id) {
  const leaf = (name || '').trim().replace(/\.(?:md|markdown|txt)$/i, '') || `ticket-${id}`
  return `qa/${leaf}.md`
}

/**
 * The other direction: what to put in the name box for a document that already exists.
 *
 * The path *inside* `qa/`, so `docPath` takes it straight back — folders included. Seeding the
 * box with the file name alone loses them, and then the box disagrees with the document it was
 * seeded from: the hint reads "that is a rename" before the BA has typed anything, and saving
 * a correction would quietly move `qa/business/pricing-2026.md` to `qa/pricing-2026.md`.
 * @param {string} path the document's stored path, or ""
 * @returns {string} the name, ready to edit
 */
export function docName(path) {
  return (path || '').replace(/^qa\//i, '')
}

/** WrongPass means the password was refused; anything else is a real failure. */
export class WrongPass extends Error {}

export const pass = () => sessionStorage.getItem(KEY) || ''
export function setPass(v) {
  try {
    v ? sessionStorage.setItem(KEY, v) : sessionStorage.removeItem(KEY)
  }
  catch {
    /* private mode — the password just isn't remembered */
  }
}

/** @returns {Promise<{tickets: object[], open: number, answered: number, confirmed: number, rejected: number}>} */
export async function queue() {
  return json('/api/tickets')
}

/**
 * How many tickets exist, which is not how many arrived.
 *
 * `db.Queue` counts the whole table for the four totals and then lists with `LIMIT 100`, so
 * the payload already carries both numbers and nothing has to be guessed from the array's
 * length. The screen needs the difference: paging what arrived is honest only while the
 * reader can see that row 101 was never sent — a pager whose last page is not the last
 * ticket is the same lie as no pager, told more convincingly.
 * @param {{open: number, answered: number, confirmed: number, rejected: number}} q the payload
 * @returns {number} every ticket in the table, listed or not
 */
export function queueTotal(q) {
  return q.open + q.answered + q.confirmed + q.rejected
}

/** File a gap. `miss` is what the engine answered instead — the BA's evidence. */
export async function file(question, miss) {
  return json('/api/tickets', { method: 'POST', body: { question, miss } })
}

/**
 * Move a ticket: draft | confirm | retract | reject. Requires the BA password.
 *
 * `confirm` on an already-confirmed ticket republishes it, which is how an answer is
 * corrected: the document path comes from the id, so the fix lands where the citation
 * already points.
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 */
export async function act(id, action, body = {}) {
  return json(`/api/tickets/${id}/${action}`, { method: 'POST', body, auth: true })
}

/**
 * Drop the ticket itself. The answer's text survives as a document row — removal is a
 * `deleted_at` column — so this costs the question and its history, never the words.
 * @param {number} id the ticket to remove
 * @returns {Promise<{id: number}>} the id that was removed
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 */
export async function drop(id) {
  return json(`/api/tickets/${id}`, { method: 'DELETE', auth: true })
}

/** Answers still free to replay, most recently used first. */
export async function history() {
  return json('/api/history')
}

async function json(url, { method = 'GET', body, auth = false } = {}) {
  const headers = {}
  if (body)
    headers['Content-Type'] = 'application/json'
  if (auth)
    headers['X-BA-Pass'] = pass()

  let res
  try {
    res = await fetch(url, { method, headers, body: body ? JSON.stringify(body) : undefined })
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  // 401 wrong password · 403 this instance has no write surface. Both mean "don't
  // retry with the same secret", and the message is the server's own.
  if (res.status === 401 || res.status === 403) {
    setPass('')
    throw new WrongPass((await res.text()).trim() || 'Not allowed')
  }
  if (!res.ok)
    throw new Error((await res.text()).trim() || `Server error ${res.status}`)
  return res.json()
}
