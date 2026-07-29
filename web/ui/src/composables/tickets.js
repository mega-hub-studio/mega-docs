/* ══ use/tickets.js — moving a ticket through the four states ══════════════════
   draft · confirm · retract · reject · delete, and the answer being typed while it happens.

   One path for every transition, so each outcome is handled once: the toast, the refused
   password, and telling the shell that what it holds is now stale. Splitting them into a
   method each is how the reject path ends up without the "it stays on the list, with your
   reason" message that makes a dismissal readable.

   The drafts live here, not on the server's copy of the ticket: someone typing an answer
   must not have it overwritten by the next queue refresh.

   Two pieces of state exist only because `confirmed` stopped being a dead end:

   · `editing` — which published answer has its box open. A correction is a confirm with new
     text, so the card needs to know whether to show the record or the editor, and that is a
     branch, which means it cannot live in the component (rule 11).
   · `armed` — which destructive button has been pressed once. Delete and dismiss act on the
     press they receive, and they sit on a card a BA scrolls past on a phone, so the first
     press only arms and the label says so. Same shape as the library's REMOVE, deliberately:
     one confirmation idiom in the app, not two.
   ═══════════════════════════════════════════════════════════════════════════ */
import { reactive, ref, watch } from 'vue'
import * as qa from '../lib/qa.js'

/* What each transition means, said once, in the words of what just happened rather than the
   verb that did it. A confirm is the only one worth celebrating: it is the moment a gap
   became part of the documents.

   `title` and not `<b>` in the message: `toast(msg)` renders msg as *text* — markup needs
   `{html: true}`, which every call in this app had quietly skipped, so a toast that meant to
   emphasise its first three words printed the tag names instead. `title` is the recipe for
   this exact shape, and it cannot inject the way an interpolated `html: true` could. */
const TOASTS = {
  draft: t => ['Draft saved', `Ticket #${t.id} is not published yet.`],
  confirm: t => ['In the knowledge base', `${t.doc} — the next question retrieves it.`],
  retract: t => ['Taken back out', `#${t.id} is a draft again and answers nothing until you confirm it.`],
  reject: t => [`Dismissed #${t.id}`, 'It stays on the list, with your reason.'],
}

/* The destructive pair, and what the button says on the press that arms it. A label is the
   only thing standing between a scrolling thumb and a document leaving the corpus, so it
   states the consequence rather than asking a question. */
export const ARMED_LABEL = {
  reject: 'DISMISS — SURE?',
  delete: 'DELETE — SURE?',
}

/**
 * @param {{ tickets: () => object[], toast: Function, onMoved: (t: object|null) => void,
 *   onLocked: (e: Error) => void }} deps
 *   tickets is a getter over the queue the shell owns; onMoved carries the ticket only for
 *   a confirm, which is the one the chat thread has to reflect.
 */
export function useTickets({ tickets, toast, onMoved, onLocked }) {
  const drafts = reactive({}) // ticket id → the answer being typed
  const working = ref(0) // id of the ticket currently being published
  const editing = ref(0) // id of the confirmed ticket whose answer is open for correction
  const armed = ref('') // `${id}:${action}` of the destructive button pressed once

  // Seed each editor from the server's copy, and keep doing it as the queue refreshes —
  // without overwriting an answer someone is halfway through typing.
  watch(
    tickets,
    (list) => {
      for (const t of list) {
        if (drafts[t.id] === undefined)
          drafts[t.id] = t.answer || ''
      }
    },
    { immediate: true },
  )

  /** Open a published answer for correction, starting from the text that is live. */
  function edit(ticket) {
    drafts[ticket.id] = ticket.answer || ''
    armed.value = ''
    editing.value = ticket.id
  }

  /** Put the editor away, discarding the correction rather than the published answer. */
  function cancel(ticket) {
    drafts[ticket.id] = ticket.answer || ''
    editing.value = 0
  }

  /** Arm a destructive button, or disarm it if it was the one already armed. */
  function arm(ticket, action) {
    const key = `${ticket.id}:${action}`
    armed.value = armed.value === key ? '' : key
  }

  const isArmed = (ticket, action) => armed.value === `${ticket.id}:${action}`

  async function move(ticket, action) {
    working.value = ticket.id
    armed.value = '' // it is acting now: an armed label must not survive the press
    const answer = (drafts[ticket.id] || '').trim()
    try {
      const updated = await qa.act(ticket.id, action, { answer, note: answer })
      drafts[ticket.id] = updated.answer || ''
      editing.value = 0
      const [title, body] = TOASTS[action](updated)
      toast(body, { title, accent: action === 'confirm' ? 'good' : 'warn' })
      onMoved(action === 'confirm' ? updated : null)
    }
    catch (e) {
      onLocked(e)
    }
    finally {
      working.value = 0
    }
  }

  /**
   * Drop the ticket. Separate from `move` because it is the one action with no ticket to
   * come back — the row leaves the list, so there is no status to re-render and nothing to
   * seed the editor from.
   */
  async function remove(ticket) {
    working.value = ticket.id
    armed.value = ''
    try {
      await qa.drop(ticket.id)
      delete drafts[ticket.id]
      editing.value = 0
      toast('The question is gone; the answer\'s text stays in the library.', {
        title: `Deleted #${ticket.id}`,
        accent: 'warn',
      })
      onMoved(null)
    }
    catch (e) {
      onLocked(e)
    }
    finally {
      working.value = 0
    }
  }

  return { drafts, working, editing, armed, isArmed, arm, edit, cancel, move, remove }
}
