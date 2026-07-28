<script setup>
/* ══ EmptyScreen.vue — the first screen, and the only one that has to teach ══════
   Nothing has been asked yet, so this is where the app says what it knows: how much is
   indexed, what other people already asked (free to repeat), and what is with a BA.
   Three lists, all of them tappable — the design system's guidance is "always an action
   out", which on a phone means a button, not a hint.

   Props only. Every list arrives resolved: `corpus.state` already distinguishes an empty
   index from an unreachable one, because "not found in the documents" reads identically
   for both and that cost an afternoon once.
   ═══════════════════════════════════════════════════════════════════════════ */
import { shortDate } from "../lib/library.js";
import { STATUS } from "../lib/qa.js";

defineProps({
  corpus: { type: Object, required: true },
  starters: { type: Array, default: () => [] },
  history: { type: Array, default: () => [] },
  queue: { type: Object, required: true },
});

defineEmits(["ask", "replay"]);
</script>

<template>
  <!-- .empty ships the dashed panel + centred mono copy -->
  <div class="empty">
    <span class="icon">◈</span>
    <span class="title">Ask the source of truth</span>

    <!-- What's indexed, stated up front. An empty index used to be indistinguishable
         from a broken retriever: you asked, and got "not found in the documents" with no
         way to tell which it was. -->
    <p v-if="corpus.state === 'ready'">
      {{ corpus.docs }} documents · {{ corpus.chunks }} retrievable sections.
      Every claim cited.
    </p>
    <p v-else-if="corpus.state === 'empty'">
      Nothing is indexed yet — run <code>make ingest DOCS=./docs</code>, then ask.
    </p>
    <p v-else-if="corpus.state === 'unavailable'">
      Can't read the index. Check the server, then reload.
    </p>
    <p v-else>Grounded answers from approved docs — every claim cited.</p>

    <div v-if="corpus.state === 'ready'" class="suggest">
      <button v-for="s in starters" :key="s" class="suggest-item" @click="$emit('ask', s)">
        {{ s }} <nes-icon name="arrowRight" />
      </button>
    </div>

    <!-- History. Every row here is an answer already paid for: asking it again costs
         nothing, so the hit count is worth showing rather than hiding. -->
    <details v-if="history.length" class="corpus">
      <summary>
        <span class="eyebrow">Asked before ({{ history.length }}) — free to repeat</span>
      </summary>
      <!-- .suggest is the library's tap-to-send recipe, the same one the starters above
           use: a question is body copy, so it must not arrive as uppercase mono chrome
           the way a .btn label would. -->
      <div class="suggest">
        <button
          v-for="h in history" :key="h.scope + h.question" class="suggest-item"
          @click="$emit('replay', h)"
        >
          {{ h.question }}
          <!-- A scoped row is only free in its own scope, so replaying it restores that
               scope — the badge is why the folder is about to change. -->
          <span v-if="h.scope" class="badge clear" :title="`Answered from ${h.scope}`">
            {{ h.scope }}</span>
          <span v-if="h.hits" class="badge clear" :title="`${h.hits} free repeats so far`">
            {{ h.hits }}×</span>
        </button>
      </div>
    </details>

    <!-- What the team has already sent to a BA, so a gap doesn't get filed twice and the
         person who filed it can see it land. -->
    <details v-if="queue.tickets.length" class="corpus">
      <summary>
        <span class="eyebrow">
          Questions with a BA ({{ queue.open + queue.answered }} waiting)</span>
      </summary>
      <ol class="timeline">
        <li v-for="t in queue.tickets" :key="t.id" :data-accent="STATUS[t.status].accent">
          <div class="time">#{{ t.id }} · {{ shortDate(t.updated_at) }}</div>
          <div class="title">
            <span class="badge" :class="STATUS[t.status].badge">{{ STATUS[t.status].label }}</span>
          </div>
          <p>{{ t.question }}</p>
        </li>
      </ol>
    </details>
  </div>
</template>
