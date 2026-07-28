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

/** File a gap. `miss` is what the engine answered instead — the BA's evidence. */
export async function file(question, miss) {
  return json('/api/tickets', { method: 'POST', body: { question, miss } })
}

/**
 * Move a ticket: draft | confirm | reject. Requires the BA password.
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 */
export async function act(id, action, body = {}) {
  return json(`/api/tickets/${id}/${action}`, { method: 'POST', body, auth: true })
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
