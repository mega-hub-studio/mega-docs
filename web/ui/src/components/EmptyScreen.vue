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
import { useFinder } from '../composables/finder.js'
import { useT } from '../composables/lang.js'
import { coverQuestion, docTitle, shortDate } from '../lib/library.js'
import { STATUS } from '../lib/qa.js'

const props = defineProps({
  corpus: { type: Object, required: true },
  history: { type: Array, default: () => [] },
  queue: { type: Object, required: true },
})

defineEmits(['ask', 'replay'])

// A getter, not `props.corpus.documents`: the corpus object is replaced on every refresh.
const { t } = useT()
const { query, shown, extra } = useFinder({ documents: () => props.corpus.documents })
</script>

<template>
  <!-- .empty ships the dashed panel + centred mono copy -->
  <div class="empty">
    <span class="icon">◈</span>
    <span class="title">{{ t('empty.title') }}</span>

    <!-- What's indexed, stated up front. An empty index used to be indistinguishable
         from a broken retriever: you asked, and got "not found in the documents" with no
         way to tell which it was.

         .note-stats rather than a paragraph, because these are three facts and not a
         sentence: as prose inside .empty's 42ch cap they wrapped mid-phrase ("Every
         claim / cited."), which is exactly the ragged line a metric bar cannot produce. -->
    <div v-if="corpus.state === 'ready'" class="note-stats">
      <span><nes-icon name="file" /> {{ t('empty.documents', { n: corpus.docs }) }}</span>
      <span><nes-icon name="layers" /> {{ t('empty.sections', { n: corpus.chunks }) }}</span>
      <span><nes-icon name="check" /> {{ t('empty.cited') }}</span>
    </div>

    <!-- Not a paragraph either: both of these are a message with a tone and a next
         action, which is what .callout is for. -->
    <div v-else-if="corpus.state === 'empty'" class="callout warn">
      <nes-icon name="warn" />
      <!-- The command is not translated: it is typed verbatim into a shell. -->
      <i18n-t keypath="empty.nothingIndexed" tag="p" scope="global">
        <template #cmd><code>make ingest DOCS=./docs</code></template>
      </i18n-t>
    </div>
    <!-- `gotcha` is the library's --crit-accented callout. There is no `.callout.crit`:
         crit is a *badge* fill, so spelling it here fell through to the default gold and
         rendered this in almost the tone of the milder `warn` above — inverting the two
         states this pair exists to tell apart. -->
    <div v-else-if="corpus.state === 'unavailable'" class="callout gotcha">
      <nes-icon name="alertCircle" />
      <p>{{ t('empty.unavailable') }}</p>
    </div>
    <p v-else>{{ t('empty.fallback') }}</p>

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
          :placeholder="t('empty.filter')" :aria-label="t('empty.filterLabel')"
        >
        <!-- Plain .badge, not .clear: in this design system `clear` is the *good/green*
             status fill, not "quiet". On a section count it claims a pass state that has no
             meaning and renders as the loudest thing on the row. -->
        <span class="badge" :title="t('empty.showing', { shown: shown.length, total: corpus.documents.length })">
          {{ shown.length }}/{{ corpus.documents.length }}</span>
      </div>

      <!-- Ranked by retrievable sections — the count on each row is the ranking, shown
           rather than claimed. -->
      <div class="palette-list">
        <button
          v-for="d in shown" :key="d.path" class="result"
          :title="coverQuestion(d)" @click="$emit('ask', coverQuestion(d))"
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
      <p v-if="extra" class="palette-empty">{{ t('empty.moreMatch', { n: extra }) }}</p>

      <p v-if="!shown.length" class="palette-empty">
        {{ t('empty.noMatch', { q: query }) }}
      </p>
    </div>

    <!-- History. Every row here is an answer already paid for: asking it again costs
         nothing, so the hit count is worth showing rather than hiding.

         Native <details>, not <nes-collapsible>: that element rewrites its own innerHTML
         when it upgrades, which would fight Vue over the v-for list inside it. -->
    <details v-if="history.length" class="corpus">
      <summary>
        <span class="eyebrow">{{ t('empty.askedBefore', { n: history.length }) }}</span>
      </summary>
      <!-- The same .result row as the menu above, so the two lists read as one grid.
           A question is body copy, so it must not arrive as uppercase mono chrome the
           way a .btn label would.

           NOT .palette-list: the library nests that class inside `.palette`, so out here it
           matches nothing and the rows would lose the 2px rhythm they are supposed to share
           with the menu. `.results` is this app's own stack — `.result` itself is top-level
           in the library, so the rows are the library's.

           No leading icon either: it was the same glyph on every row, so it carried no
           information while costing one innerHTML parse per row (<nes-icon> builds itself in
           connectedCallback) — 20 upgrades and 6.3 kB of SVG inside a *collapsed*
           disclosure, at the server's history ceiling. -->
      <div class="results">
        <button
          v-for="h in history" :key="h.scope + h.question" class="result"
          @click="$emit('replay', h)"
        >
          <span class="result-body">
            <span class="result-title">{{ h.question }}</span>
            <!-- A scoped row is only free in its own scope, so replaying it restores that
                 scope — the path is why the folder is about to change. -->
            <span class="result-path">{{ h.scope || t('empty.wholeCorpus') }}</span>
          </span>
          <span
            v-if="h.hits" class="result-hint"
            :title="t('empty.freeRepeats', { n: h.hits })"
          >{{ h.hits }}×</span>
        </button>
      </div>
    </details>

    <!-- What the team has already sent to a BA, so a gap doesn't get filed twice and the
         person who filed it can see it land. The count is the whole list: "waiting" was
         open + answered, which reads "(0 waiting)" over a list of three the moment they
         are all confirmed.

         Every status belongs here, `confirmed` most of all — an answered question is the one
         outcome the person who filed it is waiting to see. Reported as a bug ("why is an
         IN KNOWLEDGE ticket in the waiting list?") and it was the *label*: `empty.withBa` read
         "đang chờ BA" in Vietnamese, which promises a queue, while the English says "with a
         BA", which promises what this is. Do not answer that report by filtering the list —
         the filter is what the comment above is already arguing against. -->
    <details v-if="queue.tickets.length" class="corpus">
      <summary>
        <span class="eyebrow">
          {{ t('empty.withBa', { n: queue.tickets.length }) }}</span>
      </summary>
      <ol class="timeline">
        <!-- `ticket`, not `t`: the loop variable used to be `t`, which now shadows the
             translate function of the same name — so a `t('key')` added inside this loop
             would silently read a property off the ticket instead. vue/no-template-shadow
             caught it; the rename is the fix that cannot come back. -->
        <li
          v-for="ticket in queue.tickets" :key="ticket.id"
          :data-accent="STATUS[ticket.status].accent"
        >
          <div class="time">#{{ ticket.id }} · {{ shortDate(ticket.updated_at) }}</div>
          <div class="title">
            <span class="badge" :class="STATUS[ticket.status].badge">
              {{ STATUS[ticket.status].label }}</span>
          </div>
          <p>{{ ticket.question }}</p>
        </li>
      </ol>
    </details>
  </div>
</template>
