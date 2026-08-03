<script setup>
/* ══ TicketCard.vue — one ticket, in whichever of its four states it is in ═══════
   open · answered · confirmed · rejected, and the card is a different thing in each:
   a form, a form with a draft in it, a record with the document it became, or a note
   saying why not. The transitions live in composables/tickets.js — the parent owns them
   because "which ticket is working" is one value shared by the whole queue.

   The draft is `v-model:draft` rather than local state: it belongs to the queue (a BA
   switching tabs and coming back expects it), and a component that keeps its own copy of
   somebody else's state is the drift this split exists to prevent.

   ── the card is a row you open ──
   Everything below the summary is inside one `<details>`, and `open` is bound to whether
   this ticket is *work*: a question waiting for a BA and a draft being written are expanded,
   a published or dismissed one is a single 31px row until it is asked for. A queue is mostly
   settled tickets within a week of going live — measured on a phone, one confirmed ticket
   rendered its whole published answer at ~900px, so twelve of them were a scroll with no
   floor and the two open ones were somewhere inside it.

   Binding `open` rather than defaulting it is what makes a confirm read right: the status
   moves to `confirmed`, Vue patches the attribute, and the ticket folds itself away as it
   leaves the work. Retracting one unfolds it again, for the same reason and with no code.

   The question is an `<h2 class="q">` *inside* the summary — valid, since summary takes
   heading content — so the document outline still lists every question on the screen. That
   was the reason it was an h2 before this, and losing it silently is exactly the kind of
   thing a layout change takes with it.

   ── the confirmed body, and why it looks the way it does ──
   It used to be a read-only <dl>: four states, and the one a BA reaches by doing the work
   had no way out of it. The fix is four verbs, and the layout is what keeps four verbs from
   reading as four equally likely things to press:

     · EDIT ANSWER is the only button in the open. It is the common case by a distance —
       a published answer is usually almost right — and it is not destructive.
     · The other three live behind one disclosure, closed by default. A destructive control
       a thumb can reach while scrolling is a destructive control that gets pressed; one tap
       to reveal costs the deliberate user nothing and stops the accidental one entirely.
     · DISMISS and DELETE arm on the first press and act on the second, with the label
       saying which. Same idiom as the library's REMOVE — one confirmation pattern in the
       app, so learning it once is enough.
     · Each button says what happens to the reader, not which endpoint it calls. "Answers
       nothing until you confirm it" is the fact a BA needs; "retract" is our word for it.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ARMED_LABEL } from '../composables/tickets.js'
import { shortDate } from '../lib/library.js'
import { docPath, STATUS } from '../lib/qa.js'

const props = defineProps({
  ticket: { type: Object, required: true },
  draft: { type: String, default: '' },
  // What the published document will be called, inside qa/. `v-model:name` for the same
  // reason as the draft: it belongs to the queue, and a BA who switches tabs mid-thought
  // expects to find it. Blank keeps the name the ticket already has.
  name: { type: String, default: '' },
  unlocked: Boolean,
  writes: Boolean,
  // The id of the ticket currently being written, or null — one at a time, so a
  // double-tap cannot confirm twice.
  working: { type: [Number, String], default: null },
  // The id whose published answer is open for correction, and the `id:action` of the
  // destructive button pressed once. Both belong to the queue rather than the card: only
  // one ticket at a time may be either, and the parent is what can hold "one".
  editing: { type: [Number, String], default: 0 },
  armed: { type: String, default: '' },
})

defineEmits(['update:draft', 'update:name', 'move', 'edit', 'cancel', 'arm', 'remove'])

const busy = () => props.working === props.ticket.id
const correcting = () => props.editing === props.ticket.id
const isArmed = action => props.armed === `${props.ticket.id}:${action}`
// The path this confirm will write, in the hint rather than left for the BA to predict. The
// rule is lib/qa.js's, which says why it is a preview of the engine's.
const path = () => docPath(props.name, props.ticket.id)
</script>

<template>
  <article class="card ticket" :data-accent="STATUS[ticket.status].accent">
    <details class="ticket-fold" :open="ticket.status === 'open' || ticket.status === 'answered'">
      <!-- One row: what state, which ticket, when it last moved, and the question. The date
           is `updated_at` for every state — on an open ticket nothing has moved it, so it is
           the day it was asked, and on the others it is the day it was settled, which is the
           fact the record used to spend a whole <dt>/<dd> pair repeating. -->
      <summary>
        <span class="tf-meta">#{{ ticket.id }}</span>
        <span class="badge" :class="STATUS[ticket.status].badge">{{ STATUS[ticket.status].label }}</span>
        <span class="tf-meta">{{ shortDate(ticket.updated_at) }}</span>
        <h2 class="q">{{ ticket.question }}</h2>
      </summary>

      <div class="tf-body">
        <!-- The evidence: what the engine said instead. Collapsed, because the BA needs the
             question first and this only when the answer looks wrong. -->
        <details v-if="ticket.miss" class="corpus">
          <summary><span class="eyebrow">What the engine answered</span></summary>
          <div class="callout gotcha">{{ ticket.miss }}</div>
        </details>

        <template v-if="ticket.status === 'open' || ticket.status === 'answered'">
          <label v-if="unlocked" class="field">
            <span class="label">Answer from the source of truth</span>
            <textarea
              class="textarea" rows="4" :value="draft"
              placeholder="Markdown. Short and exact."
              @input="$emit('update:draft', $event.target.value)"
            />
            <span class="hint">
              Confirming writes <code>{{ path() }}</code>, indexes it, and cites it by that name.
              Dismissing uses this box as the reason.
            </span>
          </label>
          <!-- The name is the citation, so it is worth a box: `ticket-4.md` tells a reader which
               ticket and nothing about the answer, while `pricing-2026.md` tells them what they
               are about to read. Optional on purpose — a BA in a hurry should not be stopped to
               name a file, and the id is a name that always works. -->
          <label v-if="unlocked" class="field">
            <span class="label">Document name</span>
            <input
              class="input" :value="name" placeholder="pricing-2026"
              @input="$emit('update:name', $event.target.value)"
            >
            <span class="hint">Optional. Lives in <code>qa/</code>, and <code>.md</code> is added.</span>
          </label>
          <div v-if="unlocked" class="control-group">
            <button
              class="btn" :disabled="!draft || busy()" :aria-busy="busy() ? 'true' : null"
              @click="$emit('move', 'confirm')"
            >
              CONFIRM INTO KNOWLEDGE
            </button>
            <button class="btn soft" :disabled="!draft || busy()" @click="$emit('move', 'draft')">
              SAVE DRAFT
            </button>
            <button
              class="btn ghost" data-accent="crit" :disabled="busy()"
              @click="$emit('move', 'reject')"
            >
              DISMISS
            </button>
          </div>
          <p v-else-if="writes">Unlock above to answer this.</p>
        </template>

        <template v-else-if="ticket.status === 'confirmed'">
          <!-- The record, while nobody is correcting it. -->
          <dl v-if="!correcting()" class="datalist">
            <dt>Answer</dt><dd>{{ ticket.answer }}</dd>
            <dt>Document</dt><dd><code>{{ ticket.doc }}</code></dd>
          </dl>

          <!-- The correction. Same box as an unpublished answer, because it is the same
               writing job — what differs is that this text is already being read. -->
          <label v-else class="field">
            <span class="label">Correct this answer</span>
            <textarea
              class="textarea" rows="4" :value="draft"
              placeholder="Markdown. Short and exact."
              @input="$emit('update:draft', $event.target.value)"
            />
            <span class="hint">
              Saving replaces <code>{{ ticket.doc }}</code> and re-indexes it, so the next
              answer uses this text. The citation keeps pointing at the same name.
            </span>
          </label>

          <!-- Renaming a published answer, which is the same box doing a different job — so it
               says the consequence rather than repeating the label above. The old name stops
               answering the moment this saves: one answer, one address, and a reader who saved the
               old one finds nothing rather than a stale copy. -->
          <label v-if="correcting()" class="field">
            <span class="label">Document name</span>
            <input
              class="input" :value="name" placeholder="pricing-2026"
              @input="$emit('update:name', $event.target.value)"
            >
            <span class="hint">
              Saving writes <code>{{ path() }}</code>.
              <template v-if="path() !== ticket.doc">
                That is a rename — <code>{{ ticket.doc }}</code> stops being retrieved, and anything
                that quoted it by name no longer resolves. The text of both stays in the library.
              </template>
            </span>
          </label>

          <div v-if="unlocked && correcting()" class="control-group">
            <button
              class="btn" :disabled="!draft || busy()" :aria-busy="busy() ? 'true' : null"
              @click="$emit('move', 'confirm')"
            >
              SAVE CORRECTION
            </button>
            <button class="btn ghost" type="button" :disabled="busy()" @click="$emit('cancel')">
              CANCEL
            </button>
          </div>

          <div v-else-if="unlocked" class="control-group">
            <button class="btn soft" type="button" :disabled="busy()" @click="$emit('edit')">
              EDIT ANSWER
            </button>
          </div>

          <!-- Everything that un-answers the question, one tap away and closed by default.
               The summary names the outcome rather than the section, so a BA looking for the
               way out reads the words they are looking for. -->
          <details v-if="unlocked && !correcting()" class="corpus ticket-undo">
            <summary><span class="eyebrow">Take this out of the knowledge base</span></summary>
            <div class="control-group">
              <button
                class="btn soft" type="button" :disabled="busy()"
                @click="$emit('move', 'retract')"
              >
                BACK TO DRAFT
              </button>
              <button
                class="btn ghost" type="button" data-accent="crit" :disabled="busy()"
                :aria-label="isArmed('reject') ? 'Confirm: dismiss this ticket' : 'Dismiss this ticket'"
                @click="isArmed('reject') ? $emit('move', 'reject') : $emit('arm', 'reject')"
              >
                {{ isArmed('reject') ? ARMED_LABEL.reject : 'DISMISS' }}
              </button>
              <button
                class="btn ghost" type="button" data-accent="crit" :disabled="busy()"
                :aria-label="isArmed('delete') ? 'Confirm: delete this ticket' : 'Delete this ticket'"
                @click="isArmed('delete') ? $emit('remove') : $emit('arm', 'delete')"
              >
                {{ isArmed('delete') ? ARMED_LABEL.delete : 'DELETE' }}
              </button>
            </div>
            <p class="hint">
              <b>Back to draft</b> keeps your words and stops the answer being retrieved.
              <b>Dismiss</b> closes the question with a reason. <b>Delete</b> removes the
              question itself — the answer's text stays in the library either way.
            </p>
          </details>

          <p v-else-if="writes && !unlocked">Unlock above to change this.</p>
        </template>

        <!-- Dismissed, and the way out of it. The note is the whole record; DELETE is the only
             verb, because `rejected` is a resting state the store has no transition out of — a
             question dismissed in error is re-asked, which files a fresh ticket (`ticketByNorm`
             only dedupes against open and answered ones). Inline rather than behind the confirmed
             card's disclosure: a drawer holding one button is ceremony, and the two-press arming
             is what guards the press. -->
        <template v-else>
          <div class="callout memo">
            <b>Dismissed.</b> {{ ticket.note || "No reason recorded." }}
          </div>
          <div v-if="unlocked" class="control-group">
            <button
              class="btn ghost" type="button" data-accent="crit" :disabled="busy()"
              :aria-label="isArmed('delete') ? 'Confirm: delete this ticket' : 'Delete this ticket'"
              @click="isArmed('delete') ? $emit('remove') : $emit('arm', 'delete')"
            >
              {{ isArmed('delete') ? ARMED_LABEL.delete : 'DELETE' }}
            </button>
          </div>
          <p v-if="unlocked" class="hint">
            Clears the question out of the queue. Nothing else is lost — a dismissal published no
            document, so there is no text to keep.
          </p>
          <p v-else-if="writes">Unlock above to clear this.</p>
        </template>
      </div>
    </details>
  </article>
</template>
