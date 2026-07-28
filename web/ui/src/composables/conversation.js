/* ══ use/conversation.js — the thread: ask, stream, stop, regenerate, reset ═════
   The one composable with real machinery in it, and the reason the others exist: this
   used to share an object with the corpus, the queue, the status line and the diagram
   renderer, so any of them could collide with a turn mid-stream.

   Everything it needs from outside arrives as an argument, so the whole thing can be
   reasoned about from this file: the scope to ask in, where to scroll, what to do when
   an answer lands, and how to talk to the user.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref, watch } from 'vue'
import { ask as askServer } from '../lib/chat.js'
import * as session from '../lib/session.js'

let seq = 0

/**
 * One turn. `scope` is the folder it was asked in, and stays with it: a regenerate
 *  must re-ask the question that was asked, even if the reader has moved on.
 */
function newTurn(q, scope) {
  return {
    id: ++seq,
    q,
    scope,
    a: '',
    citations: [],
    streaming: true,
    error: '',
    ms: 0,
    cached: false,
    in: 0,
    out: 0,
    ticket: null, // the gap filed from this answer, once there is one
  }
}

/**
 * @param {{ scope: import("vue").Ref<string>, prompt: import("vue").Ref<Element>,
 *   scroll: (opts?: object) => void, toast: Function,
 *   onSettled: (turn: object) => void }} deps
 *   onSettled runs after every answer, however it ended — the shell uses it to refresh
 *   what an answer can have changed (the corpus, the diagram renderer, health).
 */
export function useConversation({ scope, prompt, scroll, toast, onSettled }) {
  const turns = ref(session.load()) // a reload shouldn't lose the thread
  seq = turns.value.reduce((m, t) => Math.max(m, t.id || 0), 0)
  const busy = ref(false)
  let run = null // the in-flight request, so stop() has something to abort

  // <nes-chat-prompt> flips its ▶/■ button from attributeChangedCallback, so `busy` has
  // to reach it as an *attribute*. A :busy binding won't do it: once the element sets
  // its own `busy` property, Vue's custom-element heuristic starts writing that property
  // instead and the callback stops firing, leaving the button stuck on ■.
  watch(busy, (on) => {
    const el = prompt.value
    if (!el)
      return
    if (on)
      el.setAttribute('busy', '')
    else el.removeAttribute('busy')
  })

  // Persisting on every mutation would write once per streamed token; save() debounces,
  // so a deep watcher is cheap here and never misses the last one.
  watch(turns, list => (list.length ? session.save(list) : session.clear()), { deep: true })

  async function ask(question) {
    if (!question?.trim() || busy.value)
      return
    turns.value.push(newTurn(question.trim(), scope.value))
    // Stream into the *reactive* turn, not the object just pushed: Vue hands out a proxy
    // per array item, and writing to the raw one updates no DOM.
    const turn = turns.value.at(-1)
    scroll({ force: true, smooth: true })
    await stream(turn)
  }

  /**
   * Re-run a turn in place — the mobile alternative to retyping. Always spends a real
   *  call: the user asked again because the cached answer was wrong.
   */
  async function regenerate(turn) {
    if (busy.value)
      return
    Object.assign(turn, { a: '', citations: [], error: '', ms: 0, streaming: true, cached: false, in: 0, out: 0 })
    await stream(turn, { fresh: true })
  }

  function stop() {
    run?.stop()
  }

  function reset() {
    stop()
    turns.value = []
    session.clear()
    prompt.value?.focus()
  }

  async function copy(turn) {
    try {
      await navigator.clipboard.writeText(turn.a)
      toast('<b>Copied.</b> Answer on the clipboard.', { accent: 'good' })
    }
    catch {
      toast('<b>Copy blocked.</b> Select the text instead.', { accent: 'warn' })
    }
  }

  /** Reflect a confirm on the DEV side without a reload. */
  function markConfirmed(ticket) {
    for (const t of turns.value) {
      if (t.ticket?.id === ticket.id)
        t.ticket = ticket
    }
  }

  async function stream(turn, { fresh = false } = {}) {
    busy.value = true
    const started = performance.now()
    run = askServer(turn.q, {
      fresh,
      scope: turn.scope || '',
      onToken: (tok) => {
        turn.a += tok
        scroll()
      },
      onCitations: c => (turn.citations = c),
      onDone: ({ cached, in: tin, out }) => Object.assign(turn, { cached, in: tin, out }),
    })
    try {
      await run.done // a stop() resolves quietly; only real errors throw
    }
    catch (e) {
      turn.error = e.message
    }
    finally {
      turn.ms = Math.round(performance.now() - started)
      turn.streaming = false
      busy.value = false
      run = null
      onSettled(turn)
    }
  }

  return { turns, busy, ask, regenerate, stop, reset, copy, markConfirmed }
}
