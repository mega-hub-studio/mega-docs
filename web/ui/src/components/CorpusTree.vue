<script setup>
/* ══ CorpusTree.vue — the corpus, as a tree ══════════════════════════════════════
   A host element and a call. The nodes, the rebuild rules and the element's own quirks
   live in composables/nestree.js — this file exists so the shell can write
   <CorpusTree :documents :scope @pick /> and stop thinking about it.

   One folder of documents is one question's worth of context, and this is the control
   that says which: pick "booking/calendar" and the next answer is retrieved from that
   subtree only, cited from it, and cached under it.
   ═══════════════════════════════════════════════════════════════════════════ */
import { useTemplateRef } from "vue";
import { useNesTree } from "../composables/nestree.js";

const props = defineProps({
  documents: { type: Array, default: () => [] },
  scope: { type: String, default: "" },
});

const emit = defineEmits(["pick"]);

useNesTree({
  host: useTemplateRef("host"),
  documents: () => props.documents,
  scope: () => props.scope,
  onPick: scope => emit("pick", scope),
});
</script>

<template>
  <!-- Empty on purpose: <nes-tree> is built imperatively from a JSON payload, so all this
       needs to be is somewhere to put it. -->
  <div ref="host" class="tree-host" />
</template>
