/* ══ use/qaloop.js — the gap → ticket → document loop, and the free answers ═════
   Both screens render this: the header badge and the "questions with a BA" list on
   ASK, the queue itself on BA. So it belongs to neither screen — it is shell state,
   and this is where it lives.

   The history sits here too, because it is the same fact from the other side: a
   question the corpus already answered, replayable for nothing.
   ═══════════════════════════════════════════════════════════════════════════ */
import { shallowRef } from 'vue'
import * as qa from '../lib/qa.js'

const EMPTY = { tickets: [], open: 0, answered: 0, confirmed: 0, rejected: 0 }

/**
 * @param {{ toast: Function }} deps
 * @returns {{ queue: import("vue").ShallowRef<object>, history: import("vue").ShallowRef<object[]>,
 *   refresh: () => Promise<void>, file: (turn: object) => Promise<void> }}
 */
export function useQaLoop({ toast }) {
  // shallowRef for both: a ticket and a history row are read, never edited — the loop
  // re-fetches instead (`refresh` below), which replaces `.value` and triggers anyway.
  // Deep reactivity here would proxy up to 100 tickets plus 20 history rows to watch for
  // writes that never happen. A ticket the BA confirms comes back from the server.
  const queue = shallowRef(EMPTY)
  const history = shallowRef([])

  /**
   * Failures stay silent on purpose: a stale badge beats an error banner thrown over
   *  a working conversation.
   */
  async function refresh() {
    try {
      queue.value = await qa.queue()
      history.value = await qa.history()
    }
    catch {
      /* the badge, the queue and the history stay as they were */
    }
  }

  /**
   * Report the gap this answer just proved. The failed answer travels with it, so the
   *  BA can see whether the documents are wrong or merely silent.
   */
  async function file(turn) {
    try {
      const ticket = await qa.file(turn.q, turn.error || turn.a)
      turn.ticket = ticket
      toast(`<b>Ticket #${ticket.id}.</b> A BA will answer this, and the answer joins the documents.`, {
        accent: 'good',
      })
      refresh()
    }
    catch (e) {
      toast(`<b>Couldn't file it.</b> ${e.message}`, { accent: 'crit' })
    }
  }

  return { queue, history, refresh, file }
}
