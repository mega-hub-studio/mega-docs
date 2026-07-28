<script setup>
/* ══ App.vue — the shell: what the screens share, and how it is wired ═══════════
   This file is wiring. Every piece of state lives in a composable under composables/,
   every screen is a component, and transport/rendering stay in lib/ (chat · answer · qa ·
   library · session · viewport · diagram · upload). What is left here is the product read
   out loud:

     ask · regenerate · stop · copy · reset          the conversation
     askBA · baChanged · replay                      the loop that fills the gaps
     setScope · pickScope                            which folder answers
     setMode                                         which screen you are on

   Two modes, one screen:
     DEV  asks the source of truth. When the answer is wrong or missing, one tap files
          the gap as a ticket, with the failed answer attached as evidence.
     BA   works that queue. Confirming an answer writes it into the corpus, where the
          next DEV retrieves it with a citation — and the second time anyone asks, it
          comes from the cache and costs nothing.

   Every composable is destructured, so the template names plain values and Vue unwraps
   them: `turns`, not `chat.turns.value`. The template holds markup and questions about
   state a composable already answered — a branch that needs a variable of its own
   belongs in one of them.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { onMounted, ref, useTemplateRef } from 'vue'
import BaScreen from './components/BaScreen.vue'
import ChatTurn from './components/ChatTurn.vue'
import EmptyScreen from './components/EmptyScreen.vue'
import ScopePicker from './components/ScopePicker.vue'
import StatusLine from './components/StatusLine.vue'
import { useConversation } from './composables/conversation.js'
import { useCorpus } from './composables/corpus.js'
import { useDiagrams } from './composables/diagrams.js'
import { useT } from './composables/lang.js'
import { useQaLoop } from './composables/qaloop.js'
import { useRuntime } from './composables/runtime.js'
import { useScope } from './composables/scope.js'
import { useStatusLine } from './composables/statusline.js'
import { bindViewport } from './lib/viewport.js'

const MODE_KEY = 'ke.mode' // a BA reopening the app wants the queue, not the prompt

/* ── the DOM the app has to touch directly ── */
const prompt = useTemplateRef('prompt') // busy must reach it as an attribute
const dock = useTemplateRef('dock') // the keyboard/scroll maths measures it
const pick = useTemplateRef('pick') // the picker: closed after a scope is picked
const zoom = useTemplateRef('zoom') // <dialog>: the diagram viewer
const zoomBody = useTemplateRef('zoomBody')

/* ── state, one concern per composable ── */
const { t, lang, langs, setLang } = useT()
const { scope, setScope } = useScope()
const { corpus, refresh: refreshCorpus } = useCorpus()
const { online, writes, runtime, check, watchNetwork } = useRuntime()
const { queue, history, file: askBA, refresh: refreshQueue } = useQaLoop({ toast })
const { ready: diagramsReady, loadFor, drawn, open: openZoom, close: closeZoom }
  = useDiagrams({ zoom, zoomBody })

// viewport.js binds to a real element, so it can only exist after mount. The
// conversation is given a function rather than the object, and a scroll before mount is
// a no-op instead of a crash.
let view = null
const scroll = opts => view?.scrollToEnd(opts)

const { turns, busy, ask, regenerate, stop, reset, copy, markConfirmed } = useConversation({
  scope,
  prompt,
  scroll,
  toast,
  // Everything an answer can have changed, in one place. Cheap, and cheaper than
  // reasoning about which of them this particular answer touched.
  onSettled: (turn) => {
    if (turn.error)
      check()
    if (corpus.value.state !== 'ready')
      refreshCorpus()
    loadFor(turn.a)
  },
})

const statusLine = useStatusLine({ turns, busy, online, runtime })

const mode = ref(localStorage.getItem(MODE_KEY) === 'ba' ? 'ba' : 'dev')

onMounted(() => {
  view = bindViewport(dock.value)
  check()
  refreshCorpus()
  refreshQueue()
  if (turns.value.length) {
    scroll({ force: true })
    loadFor(turns.value.map(t => t.a).join('\n'))
  }
  watchNetwork()
})

/* ── intent ── */

function setMode(next) {
  mode.value = next
  localStorage.setItem(MODE_KEY, next)
  if (next === 'ba')
    refreshQueue()
}

/** Picked from the tree: close the picker too, or the answer arrives behind it. */
function pickScope(next) {
  setScope(next)
  pick.value?.close()
}

/**
 * The BA screen moved something. What changed is never only one thing: a confirm or an
 *  import changes the corpus, which invalidates the cache, which empties the history —
 *  so refresh all three instead of reasoning about it.
 */
function baChanged(ticket) {
  refreshQueue()
  refreshCorpus()
  if (ticket)
    markConfirmed(ticket)
}

/**
 * Re-ask a cached question. Free — but only in the scope it was answered in: the same
 *  words in another folder are another question, and buying a completion from a panel
 *  labelled "free to repeat" is a broken promise.
 */
function replay(entry) {
  setMode('dev')
  setScope(entry.scope || '')
  ask(entry.question)
}
</script>

<template>
  <header class="bar">
    <!-- the light tracks /api/health, so it means something -->
    <span
      class="badge clear" :data-accent="online ? 'good' : 'crit'"
      :aria-label="online ? t('app.online') : t('app.offline')"
    >●</span>
    <!-- The mark: a stack of documents, which is what this is. Muted on purpose —
         .eyebrow's own treatment — because the only green in this bar means something
         (the health dot, the selected mode) and a logo is not a status. -->
    <span class="brand">
      <nes-icon name="layers" aria-hidden="true" />
      <span class="eyebrow">{{ t('app.brand') }}</span>
    </span>

    <!-- Two jobs, one screen. .segment is the library's single-choice control:
         aria-pressed carries the state, since the fill alone isn't announced. -->
    <div class="segment grow" role="group" :aria-label="t('app.mode')">
      <button type="button" :aria-pressed="String(mode === 'dev')" @click="setMode('dev')">
        {{ t('app.ask') }}
      </button>
      <button type="button" :aria-pressed="String(mode === 'ba')" @click="setMode('ba')">
        {{ t('app.ba') }}<template v-if="queue.open"> · {{ queue.open }}</template>
      </button>
    </div>

    <!-- EN / VI. With two languages the honest control is a button showing the one you are
         not in, not a dropdown of one alternative — and it is a real <button>, so it is
         reachable by keyboard and announced. The choice is stored under the same key the
         guide's pages use, so switching here follows a reader to the docs and back.

         `lang` is the locale, so it is displayed uppercased by CSS rather than by a second
         string: the catalogues would otherwise need EN/VI entries that are the same two
         letters in both. -->
    <button
      v-for="other in langs.filter(l => l !== lang)" :key="other"
      class="btn ghost sm lang" type="button" :aria-label="`${t('app.language')}: ${other}`"
      @click="setLang(other)"
    >
      {{ other }}
    </button>

    <button
      v-if="turns.length && mode === 'dev'" class="btn ghost icon sm"
      :aria-label="t('app.newQuestion')" @click="reset"
    >
      <nes-icon name="plus" />
    </button>
  </header>

  <main v-if="mode === 'dev'">
    <EmptyScreen
      v-if="!turns.length"
      :corpus="corpus" :history="history" :queue="queue"
      @ask="ask" @replay="replay"
    />

    <ChatTurn
      v-for="turn in turns" :key="turn.id"
      :turn="turn" :diagrams-ready="diagramsReady"
      @copy="copy" @regenerate="regenerate" @ask-ba="askBA"
      @diagram-drawn="drawn" @zoom-diagram="openZoom"
    />
  </main>

  <BaScreen
    v-else :writes="writes" :online="online" :queue="queue"
    :documents="corpus.documents" @changed="baChanged" @ask="setMode('dev')"
  />

  <!-- ══ Diagram viewer ══════════════════════════════════════════════════════
       One dialog for every diagram in the thread: a <dialog class="modal"> is the
       library's recipe, so focus trapping, the backdrop and Escape are the platform's
       rather than ours. Clicking the backdrop closes it — on a native dialog that click
       lands on the dialog element itself, which is what .self matches.
       ═══════════════════════════════════════════════════════════════════ -->
  <dialog ref="zoom" class="modal diagram-zoom" @click.self="closeZoom" @close="closeZoom">
    <div class="head">
      <span class="title">Diagram</span>
      <button class="btn ghost icon sm" aria-label="Close" @click="closeZoom">
        <nes-icon name="close" />
      </button>
    </div>
    <!-- .mermaid-view is the library's own diagram frame: panel background, square
         corners on the nodes, and scrolling. The copy is styled exactly like the
         original because it is in the same kind of box. -->
    <div ref="zoomBody" class="mermaid-view zoom-view" />
  </dialog>

  <!-- The prompt belongs to asking. In BA mode there is nothing to send, so it goes away
       rather than sitting there disabled. -->
  <div v-show="mode === 'dev'" ref="dock" class="dock">
    <ScopePicker
      v-if="corpus.documents.length" ref="pick"
      :documents="corpus.documents" :docs="corpus.docs" :scope="scope"
      @pick="pickScope" @clear="setScope('')"
    />

    <!-- <nes-chat-prompt> is the library's prompt element: it owns the growing textarea,
         Enter-to-send, and the send/stop button (busy → red ■). The app only listens for
         nes:submit / nes:stop, so none of that is reimplemented.
         `busy` reaches it as an attribute via a watcher in useConversation, not :busy. -->
    <nes-chat-prompt
      ref="prompt"
      placeholder="Ask the documents…" aria-label="Question"
      @nes:submit="ask($event.detail.value)" @nes:stop="stop"
    />

    <StatusLine :line="statusLine" :model="runtime.model" />
  </div>
</template>
