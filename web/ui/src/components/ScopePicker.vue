<script setup>
/* ══ ScopePicker.vue — which folder answers the next question ════════════════════
   Lives in the dock, not on the empty screen: it is both the answer to "does it know
   about my file?" and the control that narrows the next question, and both are needed
   *after* the first answer as often as before it. The closed row is the filter made
   visible where the question is typed — a filter you cannot see from the prompt is one
   you forget, and then read a narrow answer as the whole truth.

   `close()` is exposed because the shell closes it after a pick: the answer would
   otherwise arrive behind an open panel. A component that owns a <details> owns closing
   it, rather than handing the element out.
   ═══════════════════════════════════════════════════════════════════════════ */
import { useTemplateRef } from "vue";
import CorpusTree from "./CorpusTree.vue";

defineProps({
  documents: { type: Array, default: () => [] },
  docs: { type: Number, default: 0 },
  scope: { type: String, default: "" },
});

defineEmits(["pick", "clear"]);

const panel = useTemplateRef("panel");
// No guard: the shell calls this through `pick.value?.close()`, so it only ever runs on a
// mounted component, and a mounted component always has its own root element.
defineExpose({ close: () => (panel.value.open = false) });
</script>

<template>
  <details ref="panel" class="scopepick">
    <summary>
      <span v-if="scope" class="badge warn">
        <nes-icon name="layers" aria-hidden="true" /> {{ scope }}</span>
      <span v-if="scope" class="scope-note">answers come from this folder only</span>
      <span v-else class="scope-note">{{ docs }} documents — tap to ask inside one folder</span>
    </summary>
    <button
      v-if="scope" class="btn ghost xs"
      aria-label="Ask the whole corpus again" @click="$emit('clear')"
    >
      ALL DOCS
    </button>
    <CorpusTree :documents="documents" :scope="scope" @pick="$emit('pick', $event)" />
  </details>
</template>
