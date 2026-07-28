<script setup>
/* ══ StatusLine.vue — the ambient strip under the prompt ═════════════════════════
   The library's bottom status line: a run-state light, then what the last answer
   actually was. It lives inside the dock so viewport.js keeps measuring one height
   (--dock-h) and the conversation still clears it.

   Every field is measured, never estimated. Tokens come from the provider's own usage
   frame; the percentage only appears when CONTEXT_WINDOW says what the window is; the
   cost only appears when PRICE_IN / PRICE_OUT say what a token costs. A cached answer
   says "free" because it is — no completion was bought. Unknown prints nothing at all,
   because a zero would read as a fact.

   Which is why this component has no logic: useStatusLine decided all of it, and every
   binding here is `v-if="line.x"` — present or absent, never zero.
   ═══════════════════════════════════════════════════════════════════════════ */
defineProps({
  line: { type: Object, required: true },
  model: { type: String, default: '' },
})
</script>

<template>
  <div class="statusline" :data-state="line.state" role="status" aria-live="polite">
    <span class="sl-state">{{ line.label }}</span>
    <span v-if="model" class="sl-item" :title="`Chat model: ${model}`">
      <nes-icon name="cpu" aria-hidden="true" />{{ model }}
    </span>
    <span
      v-if="line.tokens" class="sl-item"
      title="Prompt + completion tokens the provider reported"
    >
      <nes-icon name="layers" aria-hidden="true" />{{ line.tokens }}
    </span>
    <span v-if="line.refs" class="sl-item" title="Sources this answer cited">
      <nes-icon name="link" aria-hidden="true" />{{ line.refs }}
    </span>
    <span class="sl-end">
      <span v-if="line.elapsed" class="sl-item">{{ line.elapsed }}</span>
      <span v-if="line.cost" class="sl-item" :title="line.costTitle">{{ line.cost }}</span>
    </span>
  </div>
</template>
