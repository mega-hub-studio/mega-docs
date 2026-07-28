/* ══ admin.js — the Admin screen's one request ═════════════════════════════════
   Hides: the password header, and the two answers that are not failures —

     403  this instance has no admin surface (ADMIN_PASS unset). Not a wrong password:
          there is no password that would work, so the form must not invite a retry.
     404  the route was never registered, which is the same fact reached earlier. The
          screen reads /api/health first and should never get here, so this is the
          belt to that braces.

     const settings = await load();   // [{group,name,value,source,secret}]

   Its own password, not the BA one. The list carries the provenance of every secret on
   the box, so "may publish an answer" and "may read which passwords exist" are
   deliberately different permissions — internal/server/gate.go is the other half.
   ═══════════════════════════════════════════════════════════════════════════ */

const KEY = 'mega-docs:admin-pass'

/** Session storage, like the BA pass: a tab, not a device. */
export const pass = () => sessionStorage.getItem(KEY) || ''

export function setPass(v) {
  if (v)
    sessionStorage.setItem(KEY, v)
  else sessionStorage.removeItem(KEY)
}

/** Refused means "not this password"; Absent means "no such surface here". */
export class Refused extends Error {}
export class Absent extends Error {}

/**
 * Every knob, with where its value came from.
 *
 * @throws {Refused} wrong or missing password — the form asks again
 * @throws {Absent} ADMIN_PASS is unset on this instance — the form must not ask again
 * @returns {Promise<{group: string, name: string, value: string, source: string,
 *   secret: boolean}[]>}
 */
export async function load() {
  let res
  try {
    res = await fetch('/api/settings', { headers: { 'X-Admin-Pass': pass() } })
  }
  catch {
    throw new Error('Can\'t reach the server')
  }
  if (res.status === 401) {
    setPass('')
    throw new Refused((await res.text()).trim() || 'Not allowed')
  }
  if (res.status === 403 || res.status === 404)
    throw new Absent('This instance has no admin screen: ADMIN_PASS is not set.')
  if (!res.ok)
    throw new Error((await res.text()).trim() || `Server error ${res.status}`)
  return res.json()
}

/**
 * Does this password open the gate? The list itself is the probe — there is no cheaper
 * request behind this password, and asking for the thing you want is one round trip
 * instead of two. Same reason `upload.verify` exists as its own call and this does not.
 */
export async function verify(candidate) {
  const previous = pass()
  setPass(candidate)
  try {
    await load()
    return true
  }
  catch (e) {
    setPass(previous)
    if (e instanceof Refused)
      return false
    throw e
  }
}
