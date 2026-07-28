/* ══ chat.js — the /api/chat client ═══════════════════════════════════════════
   Hides: fetch wiring, SSE framing, chunk-boundary buffering, and abort.

   One export, one shape:

     const run = ask("why is X?", { onToken, onCitations });
     run.stop();          // user pressed ■ — resolves quietly, never throws
     await run.done;      // rejects only on a *real* failure

   Callers never see an AbortController, a TextDecoder, or an "event:" line.
   ═══════════════════════════════════════════════════════════════════════════ */

/** @typedef {{ n: number, doc: string, heading: string }} Citation */

/**
 * Ask one question and stream the answer.
 * @param {string} question
 * @param {{ onToken?: (t: string) => void, onCitations?: (c: Citation[]) => void,
 *          onDone?: (info: { cached: boolean }) => void, fresh?: boolean,
 *          scope?: string }} handlers
 *   fresh skips the server's answer cache — what Regenerate means. scope narrows
 *   retrieval to one document or folder; "" is the whole corpus.
 * @returns {{ done: Promise<void>, stop: () => void }}
 */
export function ask(question, { onToken, onCitations, onDone, fresh = false, scope = '' } = {}) {
  const ctrl = new AbortController()
  const done = run(question, { onToken, onCitations, onDone, fresh, scope }, ctrl.signal)
  return { done, stop: () => ctrl.abort() }
}

async function run(question, handlers, signal) {
  let res
  try {
    res = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question, fresh: handlers.fresh, scope: handlers.scope }),
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
      return { online: false, writes: false }
    const body = await res.json()
    // The runtime fields are what the status line reports. Absent or zero means
    // "unknown", and the strip prints nothing rather than a zero — an unmeasured
    // cost and a cost of nothing are different facts.
    return {
      online: true,
      writes: !!body.writes,
      model: body.model || '',
      window: body.window || 0,
      priceIn: body.price_in || 0,
      priceOut: body.price_out || 0,
    }
  }
  catch {
    return { online: false, writes: false, model: '', window: 0, priceIn: 0, priceOut: 0 }
  }
}
