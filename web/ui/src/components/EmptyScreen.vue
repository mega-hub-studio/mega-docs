<script setup>
/* ══ EmptyScreen.vue — the first screen, and the only one that has to teach ══════
   Nothing has been asked yet, so this is where the app says what it knows: how much is
   indexed, which documents can answer, what other people already asked (free to repeat),
   and what is with a BA.

   Every block here is a library recipe: `.empty` is the panel, `.note-stats` the metric
   bar, `.palette` + `.result` the document menu, `.callout` the two states that are a
   message rather than a list, `.timeline` the BA queue. The app contributes placement and
   nothing else.

   Props and one composable. Every list arrives resolved: `corpus.state` already
   distinguishes an empty index from an unreachable one, because "not found in the
   documents" reads identically for both and that cost an afternoon once.
   ═══════════════════════════════════════════════════════════════════════════ */
import { useFinder } from "../composables/finder.js";
import { coverQuestion, docTitle, shortDate } from "../lib/library.js";
import { STATUS } from "../lib/qa.js";

const props = defineProps({
  corpus: { type: Object, required: true },
  history: { type: Array, default: () => [] },
  queue: { type: Object, required: true },
});

defineEmits(["ask", "replay"]);

// A getter, not `props.corpus.documents`: the corpus object is replaced on every refresh.
const { query, matches, shown } = useFinder({ documents: () => props.corpus.documents });
</script>

<template>
  <!-- .empty ships the dashed panel + centred mono copy -->
  <div class="empty">
    <span class="icon">◈</span>
    <span class="title">Ask the source of truth</span>

    <!-- What's indexed, stated up front. An empty index used to be indistinguishable
         from a broken retriever: you asked, and got "not found in the documents" with no
         way to tell which it was.

         .note-stats rather than a paragraph, because these are three facts and not a
         sentence: as prose inside .empty's 42ch cap they wrapped mid-phrase ("Every
         claim / cited."), which is exactly the ragged line a metric bar cannot produce. -->
    <div v-if="corpus.state === 'ready'" class="note-stats">
      <span><nes-icon name="file" /> {{ corpus.docs }} documents</span>
      <span><nes-icon name="layers" /> {{ corpus.chunks }} retrievable sections</span>
      <span><nes-icon name="check" /> every claim cited</span>
    </div>

    <!-- Not a paragraph either: both of these are a message with a tone and a next
         action, which is what .callout is for. -->
    <div v-else-if="corpus.state === 'empty'" class="callout warn">
      <nes-icon name="warn" />
      <p>Nothing is indexed yet — run <code>make ingest DOCS=./docs</code>, then ask.</p>
    </div>
    <div v-else-if="corpus.state === 'unavailable'" class="callout crit">
      <nes-icon name="alertCircle" />
      <p>Can't read the index. Check the server, then reload.</p>
    </div>
    <p v-else>Grounded answers from approved docs — every claim cited.</p>

    <!-- The document menu. Was three pre-built sentences in a `.suggest` wrap, which is
         the library's recipe for short follow-up *chips*: given sentence-length labels it
         sized each pill to its text and wrapped them, so three rows of different widths
         landed 1-then-2 on a phone. Same misuse as `.steps` for numbered instructions.

         .palette + .result is the recipe that fits: every row is one geometry — fixed
         icon column, title that ellipses instead of wrapping, path under it, count
         flush right — so a long document name can never change the shape of a row. And
         the list is the whole corpus, not the top three: with seven documents indexed,
         four of them had no row and no way to be reached. -->
    <div v-if="corpus.state === 'ready'" class="palette">
      <div class="palette-input">
        <nes-icon name="search" />
        <input
          v-model="query" type="search" autocomplete="off" spellcheck="false"
          placeholder="Filter documents…" aria-label="Filter the indexed documents"
        >
        <!-- Plain .badge, not .clear: in this design system `clear` is the *good/green*
             status fill, not "quiet". On a section count it claims a pass state that has no
             meaning and renders as the loudest thing on the row. -->
        <span class="badge" :title="`Showing ${shown.length} of ${corpus.documents.length} indexed documents`">
          {{ shown.length }}/{{ corpus.documents.length }}</span>
      </div>

      <!-- Ranked by retrievable sections — the count on each row is the ranking, shown
           rather than claimed. -->
      <div class="palette-list">
        <button
          v-for="d in shown" :key="d.path" class="result"
          :title="`Ask what ${docTitle(d)} covers`" @click="$emit('ask', coverQuestion(d))"
        >
          <nes-icon class="result-icon" name="file" />
          <span class="result-body">
            <span class="result-title">{{ docTitle(d) }}</span>
            <span class="result-path">{{ d.path }}</span>
          </span>
          <span class="result-hint">{{ d.chunks }} §</span>
        </button>
      </div>

      <!-- Never truncate in silence: when the corpus is bigger than the menu, say so and
           name the way to reach the rest. -->
      <p v-if="matches.length > shown.length" class="palette-empty">
        {{ matches.length - shown.length }} more match — type to narrow.
      </p>

      <p v-if="!matches.length" class="palette-empty">
        No indexed document matches “{{ query }}”.
      </p>
    </div>

    <!-- History. Every row here is an answer already paid for: asking it again costs
         nothing, so the hit count is worth showing rather than hiding.

         Native <details>, not <nes-collapsible>: that element rewrites its own innerHTML
         when it upgrades, which would fight Vue over the v-for list inside it. -->
    <details v-if="history.length" class="corpus">
      <summary>
        <span class="eyebrow">Asked before ({{ history.length }}) — free to repeat</span>
      </summary>
      <!-- The same .result row as the menu above, so the two lists align to one grid.
           A question is body copy, so it must not arrive as uppercase mono chrome the
           way a .btn label would. -->
      <div class="palette-list">
        <button
          v-for="h in history" :key="h.scope + h.question" class="result"
          @click="$emit('replay', h)"
        >
          <nes-icon class="result-icon" name="refresh" />
          <span class="result-body">
            <span class="result-title">{{ h.question }}</span>
            <!-- A scoped row is only free in its own scope, so replaying it restores that
                 scope — the path is why the folder is about to change. -->
            <span class="result-path">{{ h.scope || "whole corpus" }}</span>
          </span>
          <span
            v-if="h.hits" class="result-hint"
            :title="`${h.hits} free repeats so far`"
          >{{ h.hits }}×</span>
        </button>
      </div>
    </details>

    <!-- What the team has already sent to a BA, so a gap doesn't get filed twice and the
         person who filed it can see it land. The count is the whole list: "waiting" was
         open + answered, which reads "(0 waiting)" over a list of three the moment they
         are all confirmed. -->
    <details v-if="queue.tickets.length" class="corpus">
      <summary>
        <span class="eyebrow">
          Questions with a BA ({{ queue.tickets.length }})</span>
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
