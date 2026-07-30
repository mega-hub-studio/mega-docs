<script setup>
/* ══ AskScreen.vue — the DEV screen: ask, and read what came back ════════════════
   The route component for `/ask`, and a contract like every other screen: what the shell
   owns arrives as props, what the reader does leaves as an event.

     props   turns · corpus · history · queue · diagramsReady
     emit    ask · replay                 nothing asked yet — the empty screen's two doors
     emit    copy · regenerate · askBa    what one answer offers
     emit    diagramDrawn · diagramStepped · zoomDiagram   a diagram inside an answer:
                                          drawn, walked one step, or opened full screen

   The thread itself stays in the shell rather than moving in here with the markup, and
   that is the whole reason this file is props-and-events instead of a composable: an
   answer must keep streaming while a BA reads the queue, and a screen the router has
   unmounted cannot hold an in-flight request. The prompt, the scope picker and the status
   line stay there too — they are the dock, which the keyboard maths binds to.
   ═══════════════════════════════════════════════════════════════════════════ */
import ChatTurn from './ChatTurn.vue'
import EmptyScreen from './EmptyScreen.vue'

defineProps({
  turns: { type: Array, default: () => [] },
  corpus: { type: Object, required: true },
  history: { type: Array, default: () => [] },
  queue: { type: Object, required: true },
  diagramsReady: Boolean,
})

// `askBa` — see ChatTurn.vue for why two capitals in a row break a kebab-case listener.
defineEmits(['ask', 'replay', 'copy', 'regenerate', 'askBa', 'diagramDrawn', 'diagramStepped', 'zoomDiagram'])
</script>

<template>
  <main>
    <EmptyScreen
      v-if="!turns.length"
      :corpus="corpus" :history="history" :queue="queue"
      @ask="$emit('ask', $event)" @replay="$emit('replay', $event)"
    />

    <ChatTurn
      v-for="turn in turns" :key="turn.id"
      :turn="turn" :diagrams-ready="diagramsReady"
      @copy="$emit('copy', $event)" @regenerate="$emit('regenerate', $event)"
      @ask-ba="$emit('askBa', $event)" @diagram-drawn="$emit('diagramDrawn', $event)"
      @diagram-stepped="$emit('diagramStepped', $event)"
      @zoom-diagram="$emit('zoomDiagram', $event)"
    />
  </main>
</template>
