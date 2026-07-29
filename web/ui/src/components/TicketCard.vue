<script setup>
/* ══ TicketCard.vue — one ticket, in whichever of its four states it is in ═══════
   open · answered · confirmed · rejected, and the card is a different thing in each:
   a form, a form with a draft in it, a record with the document it became, or a note
   saying why not. The transitions live in composables/tickets.js — the parent owns them
   because "which ticket is working" is one value shared by the whole queue.

   The draft is `v-model:draft` rather than local state: it belongs to the queue (a BA
   switching tabs and coming back expects it), and a component that keeps its own copy of
   somebody else's state is the drift this split exists to prevent.

   ── the confirmed card, and why it looks the way it does ──
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
import { STATUS } from '../lib/qa.js'

const props = defineProps({
  ticket: { type: Object, required: true },
  draft: { type: String, default: '' },
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

defineEmits(['update:draft', 'move', 'edit', 'cancel', 'arm', 'remove'])

const busy = () => props.working === props.ticket.id
const correcting = () => props.editing === props.ticket.id
const isArmed = action => props.armed === `${props.ticket.id}:${action}`
</script>

<template>
  <article class="card ticket" :data-accent="STATUS[ticket.status].accent">
    <div class="head">
      <span class="title">#{{ ticket.id }}</span>
      <span class="badge" :class="STATUS[ticket.status].badge">{{ STATUS[ticket.status].label }}</span>
      <span class="time">{{ shortDate(ticket.asked_at) }}</span>
    </div>

    <h2 class="q">{{ ticket.question }}</h2>

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
          Confirming writes <code>docs/qa/ticket-{{ ticket.id }}.md</code>, indexes it, and
          cites it by name. Dismissing uses this box as the reason.
        </span>
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
        <dt>Confirmed</dt><dd>{{ shortDate(ticket.updated_at) }}</dd>
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

    <div v-else class="callout memo">
      <b>Dismissed.</b> {{ ticket.note || "No reason recorded." }}
    </div>
  </article>
</template>
