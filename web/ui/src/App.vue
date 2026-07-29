<script setup>
/* ══ App.vue — the shell: what the screens share, and how it is wired ═══════════
   This file is wiring. Every piece of state lives in a composable under composables/,
   every screen is a component, and transport/rendering stay in lib/ (chat · answer · qa ·
   library · session · viewport · diagram · upload). What is left here is the product read
   out loud:

     ask · regenerate · stop · copy · reset          the conversation
     askBA · baChanged · replay                      the loop that fills the gaps
     setScope · pickScope                            which folder answers

   Which screen you are on is the router's answer now, not this file's — `/ask` and `/ba`
   in the address, `router.js` for why. What is left here is the chrome both screens
   share (the header, the dock, the diagram viewer) and the state both read:

     ASK  asks the source of truth. When the answer is wrong or missing, one tap files
          the gap as a ticket, with the failed answer attached as evidence.
     BA   works that queue. Confirming an answer writes it into the corpus, where the
          next DEV retrieves it with a citation — and the second time anyone asks, it
          comes from the cache and costs nothing.

   The state stays here rather than moving into the two route components, because an
   answer has to keep streaming while a BA looks at the queue: the router unmounts a
   screen, and an unmounted screen cannot hold an in-flight request. So the router
   decides *which* screen renders and the template below decides what each is handed.

   Every composable is destructured, so the template names plain values and Vue unwraps
   them: `turns`, not `chat.turns.value`. The template holds markup and questions about
   state a composable already answered — a branch that needs a variable of its own
   belongs in one of them.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { onMounted, useTemplateRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import ReleaseModal from './components/ReleaseModal.vue'
import ScopePicker from './components/ScopePicker.vue'
import SettingsDrawer from './components/SettingsDrawer.vue'
import StatusLine from './components/StatusLine.vue'
import { useConversation } from './composables/conversation.js'
import { useCorpus } from './composables/corpus.js'
import { useDiagrams } from './composables/diagrams.js'
import { useDock } from './composables/dock.js'
import { useT } from './composables/lang.js'
import { useQaLoop } from './composables/qaloop.js'
import { useRelease } from './composables/release.js'
import { useRuntime } from './composables/runtime.js'
import { useScope } from './composables/scope.js'
import { useSettings } from './composables/settings.js'
import { useStatusLine } from './composables/statusline.js'
import { bindViewport } from './lib/viewport.js'

/* ── the DOM the app has to touch directly ── */
const prompt = useTemplateRef('prompt') // busy must reach it as an attribute
const dock = useTemplateRef('dock') // the keyboard/scroll maths measures it
const pick = useTemplateRef('pick') // the picker: closed after a scope is picked
const zoom = useTemplateRef('zoom') // <dialog>: the diagram viewer
const zoomBody = useTemplateRef('zoomBody')
const rel = useTemplateRef('rel') // <dialog>: what changed in this release

/* ── state, one concern per composable ── */
const route = useRoute() // which screen: the header, the dock and the props below read it
const { t, lang, langs, setLang } = useT()
const { scope, setScope } = useScope()
const { corpus, refresh: refreshCorpus } = useCorpus()
// The dock gets out of the way on request; `--dock-h` is measured, so main reclaims the room.
const { collapsed: dockCollapsed, toggle: toggleDock, show: showDock } = useDock()
const { online, writes, admin, runtime, check, watchNetwork } = useRuntime()
const { queue, history, file: askBA, refresh: refreshQueue } = useQaLoop({ toast })
const { ready: diagramsReady, loadFor, drawn, stepped, open: openZoom, close: closeZoom }
  = useDiagrams({ zoom, zoomBody })
// The notes are fetched on the first open and kept — they describe the binary that is
// answering, so they cannot change until it restarts.
const {
  groups: relGroups,
  meta: relMeta,
  loading: relLoading,
  error: relError,
  open: openRelease,
  close: closeRelease,
} = useRelease({ dialog: rel })

// viewport.js binds to a real element, so it can only exist after mount. The
// conversation is given a function rather than the object, and a scroll before mount is
// a no-op instead of a crash.
let view = null
const scroll = opts => view?.scrollToEnd(opts)

// The reader's own choices, and the model is the one that leaves the browser: every question
// carries it. A getter for the list, because /api/health answers after the first render.
const {
  el: settingsEl,
  open: openSettings,
  close: closeSettings,
  picked,
  current: pickedModel,
  pick: pickModel,
  muted,
  mute,
} = useSettings({ models: () => runtime.value.models })

const { turns, busy, ask, regenerate, stop, reset, copy, markConfirmed } = useConversation({
  scope,
  model: picked,
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

const statusLine = useStatusLine({ turns, busy, online, runtime, model: pickedModel })

// Arriving on a screen is when its lists are stale, and the queue is on both of them —
// the BA works it, the empty screen lists what is already filed — so refresh on every
// arrival rather than reasoning about which screen needed it.
watch(() => route.name, refreshQueue)

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

    <!-- Two jobs, one screen. .segment is the library's single-choice control, and it
         styles `> button` — so these stay real buttons and <router-link custom> renders
         no element of its own, only the navigation and whether it is the current screen.
         aria-pressed carries that state, since the fill alone isn't announced. -->
    <div class="segment grow" role="group" :aria-label="t('app.mode')">
      <router-link v-slot="{ isActive, navigate }" to="/ask" custom>
        <button type="button" :aria-pressed="String(isActive)" @click="navigate">
          {{ t('app.ask') }}
        </button>
      </router-link>
      <router-link v-slot="{ isActive, navigate }" to="/ba" custom>
        <button type="button" :aria-pressed="String(isActive)" @click="navigate">
          {{ t('app.ba') }}<template v-if="queue.open"> · {{ queue.open }}</template>
        </button>
      </router-link>
      <!-- Only where there is an admin surface to reach. A third button that answers 404 is
           worse than no button: it teaches a reader the app is broken rather than that this
           instance has no ADMIN_PASS. /api/health is what says so. -->
      <router-link v-if="admin" v-slot="{ isActive, navigate }" to="/admin" custom>
        <button type="button" :aria-pressed="String(isActive)" @click="navigate">
          {{ t('app.admin') }}
        </button>
      </router-link>
    </div>

    <!-- One gear, one panel. The language button used to live here and is in the drawer now:
         a control used twice a year was taking a quarter of a 390px bar, and settings that
         are spread across three corners are settings nobody finds. -->
    <button
      class="btn ghost icon sm" type="button" :aria-label="t('app.settings')"
      @click="openSettings"
    >
      <nes-icon name="gear" />
    </button>

    <!-- The dock comes back first, then the thread clears: `reset` focuses the prompt, and
         focusing one that is hidden puts the caret somewhere the reader cannot see. Asking
         for a new question is the one moment the box must be on screen. -->
    <button
      v-if="turns.length && route.name === 'ask'" class="btn ghost icon sm"
      :aria-label="t('app.newQuestion')" @click="showDock(); reset()"
    >
      <nes-icon name="plus" />
    </button>
  </header>

  <!-- ══ The screen ═══════════════════════════════════════════════════════════
       The router decides *which* one (router.js); this decides what each is handed,
       because the state both read lives in the shell and not in the route. Two
       branches rather than one set of bindings for both: a screen would otherwise be
       passed the other's props, which Vue would put on its root element as attributes
       and its listeners on the root as native events — `@copy` on the BA screen would
       then fire whenever a BA copies text out of a ticket.
       `route.name` rather than `Component`: before the first navigation resolves there
       is no match yet, and `:is` on nothing is a warning per render.
       ═══════════════════════════════════════════════════════════════════ -->
  <router-view v-slot="{ Component }">
    <component
      :is="Component" v-if="route.name === 'ask'"
      :turns="turns" :corpus="corpus" :history="history" :queue="queue"
      :diagrams-ready="diagramsReady"
      @ask="ask" @replay="replay" @copy="copy" @regenerate="regenerate" @ask-ba="askBA"
      @diagram-drawn="drawn" @diagram-stepped="stepped" @zoom-diagram="openZoom"
    />
    <component
      :is="Component" v-else-if="route.name === 'ba'"
      :writes="writes" :online="online" :queue="queue" :documents="corpus.documents"
      @changed="baChanged"
    />
    <!-- Read-only, so it emits nothing. Runtime and Usage are props rather than a second
         fetch: the shell already has both for the status line and the replay list. -->
    <component
      :is="Component" v-else-if="route.name === 'admin'"
      :online="online" :writes="writes" :runtime="runtime" :corpus="corpus"
      :history="history"
    />
  </router-view>

  <!-- ══ Diagram viewer ══════════════════════════════════════════════════════
       One dialog for every diagram in the thread: a <dialog class="modal"> is the
       library's recipe, so focus trapping, the backdrop and Escape are the platform's
       rather than ours. Clicking the backdrop closes it — on a native dialog that click
       lands on the dialog element itself, which is what .self matches.
       ═══════════════════════════════════════════════════════════════════ -->
  <!-- Everything a reader decides, behind the gear. `ref` lives here because the button that
       opens it is in the bar: the composable owns showModal()/close(), this owns who calls it. -->
  <SettingsDrawer
    ref="settingsEl" :models="runtime.models" :picked="picked" :current="pickedModel"
    :muted="muted" :lang="lang" :langs="langs" :admin="admin"
    :recall="turns.at(-1)?.recall ?? { kept: 0, offered: 0 }" :t="t"
    @pick="pickModel" @mute="mute" @set-lang="setLang" @close="closeSettings"
  />

  <dialog ref="zoom" class="modal diagram-zoom" @click.self="closeZoom" @close="closeZoom">
    <div class="head">
      <span class="title">Diagram</span>
      <button class="btn ghost icon sm" aria-label="Close" @click="closeZoom">
        <nes-icon name="close" />
      </button>
    </div>
    <!-- <nes-zoom> is the library's panner: wheel, drag, +/−/reset buttons and the
         keyboard, over a CSS transform. It owns `.zoom-view` and `.zoom-stage` — this
         used to borrow the *class* without the element, which is why the viewer had
         `cursor: grab` and `overflow: hidden` (both that class's) and neither a drag
         that worked nor a scroller. It moves whatever children it finds into its stage
         at upgrade time, so the frame below has to be here in the markup, not appended
         later. `.mermaid-view` is the library's diagram frame — panel, and the square
         node corners the inline copy already has.
         One thing it does not do: pinch. `.zoom-view` sets `touch-action: none` to own the
         drag, and the element listens for `wheel` and pointers only — so on a phone you
         pan with a finger and scale with its own +/− buttons. -->
    <nes-zoom aria-label="Diagram — drag to pan, scroll to zoom">
      <div ref="zoomBody" class="mermaid-view" />
    </nes-zoom>
  </dialog>

  <!-- ══ What changed ════════════════════════════════════════════════════════
       Opened from the version badge in the status line, which is only a button when a
       release was actually cut — an untagged build leaves GET /api/release unregistered and
       the badge unclickable, so this dialog cannot be reached with nothing to show.

       The <dialog> is here rather than inside ReleaseModal for the same reason the diagram
       viewer's is: the shell owns dialogs, so `useRelease` binds a real element instead of
       reaching through a component instance for its root node. The component renders the
       body. Backdrop click closes it — on a native dialog that click lands on the dialog
       element itself, which is what .self matches.
       ═══════════════════════════════════════════════════════════════════ -->
  <dialog ref="rel" class="modal release" @click.self="closeRelease" @close="closeRelease">
    <div class="head">
      <span class="title">{{ t('release.title') }}</span>
      <button class="btn ghost icon sm" :aria-label="t('release.close')" @click="closeRelease">
        <nes-icon name="close" />
      </button>
    </div>
    <ReleaseModal
      :groups="relGroups" :meta="relMeta" :loading="relLoading" :error="relError"
    />
  </dialog>

  <!-- The prompt belongs to asking. On the BA screen there is nothing to send, so it goes
       away rather than sitting there disabled. v-show, not v-if: the dock element is what
       the keyboard maths bound at mount, and it has to stay the same element. -->
  <div
    v-show="route.name === 'ask'" ref="dock" class="dock"
    :data-collapsed="dockCollapsed || null"
  >
    <!-- The handle is outside the hidden part on purpose: the control that puts the dock
         away is the control that brings it back, in the same place, so there is nothing to
         hunt for. It is the only thing left on screen when collapsed. -->
    <button
      class="dock-handle" type="button" :aria-expanded="!dockCollapsed"
      :aria-label="dockCollapsed ? 'Show the question box' : 'Hide the question box'"
      @click="toggleDock"
    >
      <nes-icon :name="dockCollapsed ? 'chevronUp' : 'chevronDown'" />
      <span>{{ dockCollapsed ? 'ASK' : 'HIDE' }}</span>
    </button>

    <!-- v-show, not v-if: <nes-chat-prompt> has to stay the same element, because
         useConversation drives its `busy` attribute through a watcher and an answer keeps
         streaming while the box that asked for it is out of sight. -->
    <div v-show="!dockCollapsed" class="dock-body">
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

      <StatusLine
        :line="statusLine" :model="picked || runtime.model" :version="runtime.version"
        :release="runtime.release" @show-release="openRelease"
        @show-settings="openSettings"
      />
    </div>
  </div>
</template>
