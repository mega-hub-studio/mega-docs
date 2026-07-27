/* ══ use/qaloop.js — the gap → ticket → document loop, and the free answers ═════
   Both screens render this: the header badge and the "questions with a BA" list on
   ASK, the queue itself on BA. So it belongs to neither screen — it is shell state,
   and this is where it lives.

   The history sits here too, because it is the same fact from the other side: a
   question the corpus already answered, replayable for nothing.
   ═══════════════════════════════════════════════════════════════════════════ */
import * as qa from "../qa.js";

const EMPTY = { tickets: [], open: 0, answered: 0, confirmed: 0, rejected: 0 };

/**
 * @param {{ toast: Function }} deps
 * @returns {{ queue: import("vue").Ref<object>, history: import("vue").Ref<object[]>,
 *   refresh: () => Promise<void>, file: (turn: object) => Promise<void> }}
 */
export function useQaLoop({ toast }) {
  const { ref } = Vue;
  const queue = ref(EMPTY);
  const history = ref([]);

  /** Failures stay silent on purpose: a stale badge beats an error banner thrown over
   *  a working conversation. */
  async function refresh() {
    try {
      queue.value = await qa.queue();
      history.value = await qa.history();
    } catch {
      /* the badge, the queue and the history stay as they were */
    }
  }

  /** Report the gap this answer just proved. The failed answer travels with it, so the
   *  BA can see whether the documents are wrong or merely silent. */
  async function file(turn) {
    try {
      const ticket = await qa.file(turn.q, turn.error || turn.a);
      turn.ticket = ticket;
      toast(`<b>Ticket #${ticket.id}.</b> A BA will answer this, and the answer joins the documents.`, {
        accent: "good",
      });
      refresh();
    } catch (e) {
      toast(`<b>Couldn't file it.</b> ${e.message}`, { accent: "crit" });
    }
  }

  return { queue, history, refresh, file };
}
