/* ══ lib/release.js — what changed, fetched when somebody asks ═══════════════════
   One request, made the first time the version badge is clicked and never on load. The
   notes grow with every commit and nobody reads them on the way to asking a question, so
   putting them in /api/health would cost every client the whole changelog on every
   reconnect. The label the badge prints comes from health; only the list lives here.

   The route is absent on an instance with no tag, and that is not an error condition — it
   is the normal state of an untagged build. It cannot be reached anyway: the badge is only
   clickable when health reported a release.
   ═══════════════════════════════════════════════════════════════════════════ */

/**
 * @typedef {object} ReleaseNote
 * @property {string} kind    Conventional-Commit type — `feat`, `fix`, …, or `other`
 * @property {string} scope   the parenthesised scope, `''` when the subject carried none
 * @property {string} subject the commit subject with its `type(scope):` prefix removed
 * @property {string} commit  the short sha, so a line can be traced back to its commit
 */

/**
 * @typedef {object} Release
 * @property {string} version  the tag, e.g. `v0.13.0`
 * @property {string} date     the tagged commit's date, `YYYY-MM-DD`
 * @property {string} previous the tag this is measured from, `''` for a first release
 * @property {ReleaseNote[]} notes    one entry per commit since `previous`, newest first
 */

/**
 * Read this instance's release notes.
 *
 * @returns {Promise<Release>} the parsed notes
 * @throws {Error} when the route is absent or the body is not the expected shape — the
 *   caller shows the message inside the dialog, because a modal that opens empty reads as
 *   a broken app rather than as a request that failed.
 */
export async function release() {
  const res = await fetch('/api/release')
  if (!res.ok)
    throw new Error(`/api/release answered ${res.status}`)
  const body = await res.json()
  return {
    version: body.version || '',
    date: body.date || '',
    previous: body.previous || '',
    // Defensive about the array only: an older binary with a newer bundle is the one
    // version skew a committed front end can actually meet.
    notes: Array.isArray(body.notes) ? body.notes : [],
  }
}
