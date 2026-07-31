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

// How many earlier exchanges ride along with a question.
//
// Twelve, not the three this was: the server trims the thread to a share of the picked
// model's window before it reaches the provider and reports what it kept, so sending fewer
// than the model can hold is a cap on memory that nothing measures and nobody chose. This
// number is the *tail worth offering*; how much of it fits is THREAD_SHARE's job, one
// layer down. It still has to be a number, because `session.js` keeps thirty and the
// oldest of those are a different conversation.
const RECALL_TURNS = 12

/**
 * The thread behind one turn, in the shape /api/chat takes: oldest first, and only turns
 * that actually have an answer. A question whose answer errored or was stopped would tell
 * the model that question went unanswered, which is not what happened.
 * @param {object[]} list every turn in the conversation
 * @param {object} turn the one being asked or regenerated — everything before it is history
 * @returns {{ q: string, a: string }[]}
 */
function threadBefore(list, turn) {
  return list
    .slice(0, list.indexOf(turn))
    .filter(t => t.a && !t.error)
    .slice(-RECALL_TURNS)
    .map(t => ({ q: t.q, a: t.a }))
}

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
    // Which model answered, and how much of the thread it read: both arrive on the `done`
    // frame. They stay with the turn because a conversation read back tomorrow has to say
    // what produced each answer — a thread where two models spoke and neither is named is a
    // thread nobody can compare.
    model: '',
    recall: { kept: 0, offered: 0 },
    // The same pair for the corpus: sections read, of sections retrieval weighed.
    retrieval: { sections: 0, candidates: 0 },
    ticket: null, // the gap filed from this answer, once there is one
  }
}

/**
 * @param {{ scope: import("vue").Ref<string>, model: import("vue").Ref<string>,
 *   prompt: import("vue").Ref<Element>,
 *   scroll: (opts?: object) => void, toast: Function,
 *   onSettled: (turn: object) => void }} deps
 *   onSettled runs after every answer, however it ended — the shell uses it to refresh
 *   what an answer can have changed (the corpus, the diagram renderer, health).
 */
export function useConversation({ scope, model, websearch, prompt, scroll, toast, onSettled }) {
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
    Object.assign(turn, {
      a: '',
      citations: [],
      error: '',
      ms: 0,
      streaming: true,
      cached: false,
      in: 0,
      out: 0,
      recall: { kept: 0, offered: 0 },
    })
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
      toast('Answer on the clipboard.', { title: 'Copied', accent: 'good' })
    }
    catch {
      toast('Select the text instead.', { title: 'Copy blocked', accent: 'warn' })
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
      // Read here rather than at the call sites, so a regenerate re-asks the follow-up
      // against the same turns it was first asked against.
      history: threadBefore(turns.value, turn),
      // The pick as it is *now*, deliberately, including on a regenerate: "answer that again
      // with the stronger model" is the whole point of having a picker beside a thread.
      model: model?.value ?? '',
      // Read here, like the model, so a regenerate re-asks under whatever the reader wants
      // *now* — "answer that again, and look outside this time" is the point of a per-question
      // switch beside a thread.
      websearch: websearch?.value ?? false,
      onToken: (tok) => {
        turn.a += tok
        scroll()
      },
      onCitations: c => (turn.citations = c),
      onDone: ({ cached, in: tin, out, model: answered, kept = 0, offered = 0, sections = 0, candidates = 0 }) =>
        Object.assign(turn, {
          cached,
          in: tin,
          out,
          model: answered || turn.model,
          recall: { kept, offered },
          retrieval: { sections, candidates },
        }),
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
