<script setup>
/* ══ TicketCard.vue — one ticket, in whichever of its four states it is in ═══════
   open · answered · confirmed · rejected, and the card is a different thing in each:
   a form, a form with a draft in it, a record with the document it became, or a note
   saying why not. The transitions live in composables/tickets.js — the parent owns them
   because "which ticket is working" is one value shared by the whole queue.

   The draft is `v-model:draft` rather than local state: it belongs to the queue (a BA
   switching tabs and coming back expects it), and a component that keeps its own copy of
   somebody else's state is the drift this split exists to prevent.
   ═══════════════════════════════════════════════════════════════════════════ */
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
})

defineEmits(['update:draft', 'move'])

const busy = () => props.working === props.ticket.id
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

    <dl v-else-if="ticket.status === 'confirmed'" class="datalist">
      <dt>Answer</dt><dd>{{ ticket.answer }}</dd>
      <dt>Document</dt><dd><code>{{ ticket.doc }}</code></dd>
      <dt>Confirmed</dt><dd>{{ shortDate(ticket.updated_at) }}</dd>
    </dl>

    <div v-else class="callout memo">
      <b>Dismissed.</b> {{ ticket.note || "No reason recorded." }}
    </div>
  </article>
</template>
