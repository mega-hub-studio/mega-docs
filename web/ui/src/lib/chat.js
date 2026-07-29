/* ══ chat.js — the /api/chat client ═══════════════════════════════════════════
   Hides: fetch wiring, SSE framing, chunk-boundary buffering, and abort.

   One export, one shape:

     const run = ask("why is X?", { onToken, onCitations });
     run.stop();          // user pressed ■ — resolves quietly, never throws
     await run.done;      // rejects only on a *real* failure

   Callers never see an AbortController, a TextDecoder, or an "event:" line.
   ═══════════════════════════════════════════════════════════════════════════ */

/** @typedef {{ n: number, doc: string, heading: string }} Citation */
/** @typedef {{ q: string, a: string }} Turn one exchange already on screen */

/**
 * Ask one question and stream the answer.
 * @param {string} question
 * @param {{ onToken?: (t: string) => void, onCitations?: (c: Citation[]) => void,
 *          onDone?: (info: { cached: boolean }) => void, fresh?: boolean,
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
  if (event === 'token')
    onToken?.(payload.t)
  else if (event === 'citations')
    onCitations?.(payload)
  else if (event === 'done')
    onDone?.({ cached: !!payload.cached, in: payload.in || 0, out: payload.out || 0 })
  else if (event === 'error')
    throw new Error(payload.message)
}

/**
 * What the server is and what it allows. Never throws — an unreachable server is
 * a state the UI shows, not an error it handles.
 * @returns {Promise<{online: boolean, writes: boolean}>} writes: a BA can confirm here
 */
export async function health() {
  try {
    const res = await fetch('/api/health')
    if (!res.ok)
      return { online: false, writes: false, admin: false }
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
    return {
      online: false,
      writes: false,
      admin: false,
      model: '',
      window: 0,
      priceIn: 0,
      priceOut: 0,
      version: '',
      release: '',
    }
  }
}
