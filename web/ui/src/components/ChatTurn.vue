<script setup>
/* ══ ChatTurn.vue — one question and its answer ═════════════════════════════════
   A contract, not a place to work: props in, events out, and the two rendering helpers
   it needs from lib/. Everything the turn *is* was decided by useConversation before it
   got here — streaming, cached, scoped, the ticket it filed — so this file only says how
   each of those reads.

   The diagram listeners live on the answer element rather than on each diagram: a
   diagram arrives inside v-html, so Vue never sees it as a component, and `nes:render`
   and `nes:step` are the library's own events and bubble to here.
   ═══════════════════════════════════════════════════════════════════════════ */
import { fileName, section, turnHtml } from '../lib/answer.js'
import { STATUS } from '../lib/qa.js'

const props = defineProps({
  turn: { type: Object, required: true },
  // Whether the 3.4 MB renderer has arrived. Until it has, a fenced diagram stays a
  // fenced diagram: <nes-mermaid> must never exist before the thing that draws it.
  diagramsReady: Boolean,
})

defineEmits(['copy', 'regenerate', 'askBA', 'diagramDrawn', 'diagramStepped', 'zoomDiagram'])

const srcId = n => `s${props.turn.id}-${n}`

// What may appear in the answer mid-stream is a rendering rule, so it lives in
// lib/answer.js with the rest of them. This says only "render this turn".
const html = () => turnHtml(props.turn, props.diagramsReady, srcId)
</script>

<template>
  <article class="turn">
    <h2 class="q">{{ turn.q }}</h2>

    <div v-if="turn.error" class="callout gotcha" role="alert">
      <b>Request failed</b> — {{ turn.error }}
    </div>

    <div v-else class="a card" data-accent="blue" :aria-busy="turn.streaming ? 'true' : null">
      <div class="head">
        <span class="title">Answer</span>
        <!-- A free answer must not look identical to a paid one, or nobody learns that
             repeating a question is cheap. -->
        <span
          v-if="turn.cached"
          class="badge todo"
          title="Served from this server's cache — no provider call, no cost"
        >CACHED</span>
        <!-- Asked inside a folder: the answer is only as complete as that folder, so the
             turn keeps saying so long after the picker moved on. -->
        <span v-if="turn.scope" class="badge warn" :title="`Retrieved from ${turn.scope} only`">
          {{ turn.scope }}</span>
        <span v-if="turn.streaming" class="spinner sm" aria-label="Generating" />
      </div>

      <!-- eslint-disable-next-line vue/no-v-html — answer.js sanitises with DOMPurify,
           which is the whole reason that module exists; see its header. -->
      <div
        class="prose"
        @nes:render="$emit('diagramDrawn', $event)"
        @nes:step="$emit('diagramStepped', $event)"
        @click="$emit('zoomDiagram', $event)"
        @keydown="$emit('zoomDiagram', $event)"
        v-html="html()"
      />

      <!-- official grounded-answer recipe: .sources rows that the inline .cite markers
           in the prose above link down to. Both halves show a leaf and keep the whole
           value in `title`: .source-host is the recipe's short secondary label — dim, mono,
           --fs-label — so a full breadcrumb in it wrapped to a second line and lost the
           indent under the filename. See lib/answer.js. -->
      <ol v-if="turn.citations.length" class="sources">
        <li v-for="c in turn.citations" :id="srcId(c.n)" :key="c.n" class="source">
          <span class="source-n">{{ c.n }}</span>
          <span class="source-title" :title="c.doc">{{ fileName(c.doc) }}</span>
          <span v-if="c.heading" class="source-host" :title="c.heading">{{
            section(c.heading)
          }}</span>
        </li>
      </ol>

      <div v-if="!turn.streaming && turn.a" class="feedback">
        <div class="feedback-actions">
          <button class="btn ghost xs icon" aria-label="Copy answer" @click="$emit('copy', turn)">
            <nes-icon name="copy" />
          </button>
          <button
            class="btn ghost xs icon"
            aria-label="Regenerate answer"
            @click="$emit('regenerate', turn)"
          >
            <nes-icon name="refresh" />
          </button>
          <!-- The whole loop starts here: one tap turns a bad answer into a question a
               BA can answer, with this answer attached as evidence. -->
          <button
            v-if="!turn.ticket"
            class="btn ghost xs"
            title="Send this question to a BA — their answer joins the documents"
            @click="$emit('askBA', turn)"
          >
            <nes-icon name="help" /> ASK BA
          </button>
        </div>
        <div class="feedback-meta">
          <span v-if="turn.citations.length">{{ turn.citations.length }} SRC</span>
          <span v-if="turn.ms">{{ turn.ms }} MS</span>
        </div>
      </div>

      <!-- Where that gap got to. The badge is the ticket's real state, refreshed when a
           BA acts, so nobody has to ask whether it was picked up. -->
      <div
        v-if="turn.ticket"
        class="callout"
        :class="turn.ticket.status === 'confirmed' ? 'tip' : 'memo'"
      >
        <span class="badge" :class="STATUS[turn.ticket.status].badge">
          {{ STATUS[turn.ticket.status].label }}</span>
        <b>#{{ turn.ticket.id }}</b> — {{ STATUS[turn.ticket.status].hint }}
        <template v-if="turn.ticket.status === 'confirmed'">
          <code>{{ turn.ticket.doc }}</code> — ask again to see it cited.
        </template>
      </div>
    </div>
  </article>
</template>
