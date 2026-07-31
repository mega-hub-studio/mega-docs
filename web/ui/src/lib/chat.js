/* ══ chat.js — the /api/chat client ═══════════════════════════════════════════
   Hides: fetch wiring, SSE framing, chunk-boundary buffering, and abort.

   One export, one shape:

     const run = ask("why is X?", { onToken, onCitations });
     run.stop();          // user pressed ■ — resolves quietly, never throws
     await run.done;      // rejects only on a *real* failure

   Callers never see an AbortController, a TextDecoder, or an "event:" line.
   ═══════════════════════════════════════════════════════════════════════════ */

/**
 * @typedef {object} Citation where one claim came from. Two kinds, two numberings: a document
 *   is `[n]` and carries `doc`/`heading`, a public search result is `[wN]` and carries
 *   `kind: "web"` with `title`/`url`. An absent `kind` is a document — every payload written
 *   before the web existed still means what it did.
 * @property {number} n the marker's number, within its own kind
 * @property {string} [doc] the document's path
 * @property {string} [heading] the section breadcrumb inside it
 * @property {string} [kind] "web", or absent for one of this organisation's documents
 * @property {string} [title] the page title, for a web result
 * @property {string} [url] the page, for a web result
 */
/** @typedef {{ q: string, a: string }} Turn one exchange already on screen */

/**
 * @typedef {object} Done the last frame of a stream, and the only one carrying facts the
 *   client cannot re-derive: `model` because a reader may switch mid-thread, `kept`/`offered`
 *   because a budget that trims silently looks like an assistant that forgot.
 * @property {boolean} cached served from the answer cache — it cost nothing
 * @property {number} in prompt tokens, 0 when the provider reported none
 * @property {number} out completion tokens, 0 when the provider reported none
 * @property {string} model which one actually answered
 * @property {number} kept turns of the thread the model read
 * @property {number} offered turns it was given to choose from
 * @property {number} sections sections of the corpus the answer was built from
 * @property {number} candidates sections retrieval weighed before the budget cut it
 */

/**
 * @typedef {object} Health what the server is and what it allows. Zero and "" mean unknown,
 *   and the UI prints nothing rather than a zero.
 * @property {boolean} online the server answered at all
 * @property {boolean} writes a BA can confirm here
 * @property {boolean} admin this instance has an admin surface
 * @property {boolean} search this instance can supplement a thin answer from the public web
 * @property {string} model the default model's name
 * @property {number} window its context window, in tokens
 * @property {{name: string, window: number, price_in: number, price_out: number}[]} models
 *   every model this instance will answer with — the picker's whole source of truth
 * @property {object} engine what it is tuned to: topK, threadShare, contextShare, cacheKeep
 * @property {number} priceIn USD per 1M prompt tokens
 * @property {number} priceOut USD per 1M completion tokens
 * @property {string} version the commit this server was built from
 * @property {string} release the tag that commit was cut from
 */

/**
 * Ask one question and stream the answer.
 * @param {string} question
 * @param {{ onToken?: (t: string) => void, onCitations?: (c: Citation[]) => void,
 *          onDone?: (info: Done) => void, fresh?: boolean,
 *          scope?: string, history?: Turn[], model?: string }} handlers
 *   fresh skips the server's answer cache — what Regenerate means. scope narrows
 *   retrieval to one document or folder; "" is the whole corpus. history is the thread
 *   this question continues, oldest first — the server holds no session, so a follow-up
 *   that arrives without it is answered as a first question.
 * @returns {{ done: Promise<void>, stop: () => void }}
 */
export function ask(question, { onToken, onCitations, onDone, fresh = false, scope = '', history = [], model = '' } = {}) {
  const ctrl = new AbortController()
  const done = run(question, { onToken, onCitations, onDone, fresh, scope, history, model }, ctrl.signal)
  return { done, stop: () => ctrl.abort() }
}

async function run(question, handlers, signal) {
  let res
  try {
    res = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        question,
        fresh: handlers.fresh,
        scope: handlers.scope,
        history: handlers.history,
        // The reader's pick, and "" means the instance default — the server refuses anything
        // it does not offer, so this is a request rather than an instruction.
        model: handlers.model,
      }),
      signal,
    })
  }
  catch (e) {
    if (e.name === 'AbortError')
      return // the user's own decision, not an error
    throw new Error('Can\'t reach the server')
  }
  if (!res.ok)
    throw new Error(`Server error ${res.status}`)

  try {
    for await (const frame of frames(res.body)) apply(frame, handlers)
  }
  catch (e) {
    if (e.name !== 'AbortError')
      throw e
  }
}

/* An SSE frame is "event: x\ndata: y" blocks separated by a blank line, and a
   network chunk can split one anywhere — so hold the tail back until it closes. */
async function* frames(body) {
  const reader = body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done)
      break
    buf += dec.decode(value, { stream: true })
    const blocks = buf.split('\n\n')
    buf = blocks.pop()
    for (const b of blocks) yield parse(b)
  }
}

function parse(block) {
  let event = 'message'
  let data = ''
  for (const line of block.split('\n')) {
    if (line.startsWith('event:'))
      event = line.slice(6).trim()
    else if (line.startsWith('data:'))
      data += line.slice(5).trim()
  }
  if (!data)
    return null
  try {
    return { event, payload: JSON.parse(data) }
  }
  catch {
    return null // a half-written frame is not worth crashing a stream over
  }
}

function apply(frame, { onToken, onCitations, onDone }) {
  if (!frame)
    return
  const { event, payload } = frame
  if (event === 'token') {
    onToken?.(payload.t)
  }
  else if (event === 'citations') {
    onCitations?.(payload)
  }
  // Every field the frame carries, not the three this once forwarded: the server omits an
  // unmeasured count and an unnamed model rather than sending a zero, so the defaults are here
  // and the layer above gets a whole `Done` either way. Dropping one is silent — a blank model
  // badge and a memory meter reading "—" are what a missing field looks like, with no error.
  else if (event === 'done') {
    onDone?.({
      cached: !!payload.cached,
      in: payload.in || 0,
      out: payload.out || 0,
      model: payload.model || '',
      kept: payload.kept || 0,
      offered: payload.offered || 0,
      sections: payload.sections || 0,
      candidates: payload.candidates || 0,
    })
  }
  else if (event === 'error') {
    throw new Error(payload.message)
  }
}

/**
 * What the server is and what it allows. Never throws — an unreachable server is
 * a state the UI shows, not an error it handles.
 * @returns {Promise<Health>}
 */
export async function health() {
  try {
    const res = await fetch('/api/health')
    if (!res.ok)
      return unknown() // a proxy's 502 mid-deploy, which is the common way this fails
    const body = await res.json()
    // The runtime fields are what the status line reports. Absent or zero means
    // "unknown", and the strip prints nothing rather than a zero — an unmeasured
    // cost and a cost of nothing are different facts.
    return {
      online: true,
      writes: !!body.writes,
      // Whether this instance has an admin surface at all. The bundle is static and cannot
      // discover which routes exist, so an unset ADMIN_PASS has to arrive as a fact rather
      // than as a 404 the Admin tab hits after someone taps it.
      admin: !!body.admin,
      // Whether this instance can reach the public web at all. A labelled outside source is
      // something a reader has to be able to expect, so the capability arrives with the rest
      // of them rather than being inferred from a badge that did or did not appear.
      search: !!body.search,
      model: body.model || '',
      window: body.window || 0,
      // Every model this instance will answer with, each with the two numbers the strip
      // needs to price and measure whichever one is picked. An older server sends none,
      // and then the four scalar fields above are the whole story — one model, no picker.
      models: Array.isArray(body.models) ? body.models : [],
      // The engine's own three numbers. Absent on an older server, and then the panel's engine
      // group is simply not there — the same rule as every other unknown here.
      engine: {
        topK: body.top_k || 0,
        threadShare: body.thread_share || 0,
        contextShare: body.context_share || 0,
        cacheKeep: body.cache_keep || 0,
      },
      priceIn: body.price_in || 0,
      priceOut: body.price_out || 0,
      // The commit this server was built from. Absent for a binary with no VCS stamp, and
      // the strip prints nothing rather than a placeholder — same rule as the prices.
      version: body.version || '',
      // The tag beside that commit, when one was cut. It is a label and not the notes: those
      // are GET /api/release, fetched only if somebody opens the dialog.
      release: body.release || '',
    }
  }
  catch {
    return unknown()
  }
}

/**
 * A server that told us nothing, as every key the success path returns. Both failure paths
 * answer with this rather than with a shorter object: the caller assigns the result wholesale,
 * so an *absent* key is not a zeroed one — it lands as `undefined` on the panel, which is how
 * the engine group read undefined for a while. One shape, so the three can never drift again,
 * and a fresh object each time because the caller is handed the array.
 * @returns {Health}
 */
function unknown() {
  return {
    online: false,
    writes: false,
    admin: false,
    search: false,
    model: '',
    window: 0,
    models: [],
    engine: { topK: 0, threadShare: 0, contextShare: 0, cacheKeep: 0 },
    priceIn: 0,
    priceOut: 0,
    version: '',
    release: '',
  }
}
