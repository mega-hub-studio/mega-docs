/* ══ use/tickets.js — moving a ticket through the four states ══════════════════
   draft · confirm · reject, and the answer being typed while it happens.

   One path for all three transitions, so every outcome is handled once: the toast, the
   refused password, and telling the shell that what it holds is now stale. Splitting them
   into three methods is how the reject path ends up without the "it stays on the list,
   with your reason" message that makes a dismissal readable.

   The drafts live here, not on the server's copy of the ticket: someone typing an answer
   must not have it overwritten by the next queue refresh.
   ═══════════════════════════════════════════════════════════════════════════ */
import * as qa from "../qa.js";

/* What each transition means, said once. A confirm is the only one worth celebrating: it
   is the moment a gap became part of the documents. */
const TOASTS = {
  draft: (t) => `<b>Draft saved.</b> Ticket #${t.id} is not published yet.`,
  confirm: (t) => `<b>In the knowledge base.</b> ${t.doc} — the next question retrieves it.`,
  reject: (t) => `<b>Dismissed #${t.id}.</b> It stays on the list, with your reason.`,
};

/**
 * @param {{ tickets: () => object[], toast: Function, onMoved: (t: object|null) => void,
 *   onLocked: (e: Error) => void }} deps
 *   tickets is a getter over the queue the shell owns; onMoved carries the ticket only for
 *   a confirm, which is the one the chat thread has to reflect.
 */
export function useTickets({ tickets, toast, onMoved, onLocked }) {
  const { ref, reactive, watch } = Vue;
  const drafts = reactive({}); // ticket id → the answer being typed
  const working = ref(0); // id of the ticket currently being published

  // Seed each editor from the server's copy, and keep doing it as the queue refreshes —
  // without overwriting an answer someone is halfway through typing.
  watch(
    tickets,
    (list) => {
      for (const t of list) {
        if (drafts[t.id] === undefined) drafts[t.id] = t.answer || "";
      }
    },
    { immediate: true },
  );

  async function move(ticket, action) {
    working.value = ticket.id;
    const answer = (drafts[ticket.id] || "").trim();
    try {
      const updated = await qa.act(ticket.id, action, { answer, note: answer });
      drafts[ticket.id] = updated.answer || "";
      toast(TOASTS[action](updated), { accent: action === "reject" ? "warn" : "good" });
      onMoved(action === "confirm" ? updated : null);
    } catch (e) {
      onLocked(e);
    } finally {
      working.value = 0;
    }
  }

  return { drafts, working, move };
}
