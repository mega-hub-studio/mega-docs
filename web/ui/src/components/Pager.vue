<script setup>
/* ══ Pager.vue — the page strip under a long list ════════════════════════════════
   A contract with nothing behind it: composables/paged.js decided every number, this
   only says how they read. `.pagination` and `.pg` are the design system's (0.16.0), so
   the arrows, the fill on the current page and the disabled ends all arrive with them —
   this file contributes the markup those two classes are written for and no CSS.

   It renders nothing at all on one page. A pager under a list that fits is chrome telling
   a reader there is nowhere to go.
   ═══════════════════════════════════════════════════════════════════════════ */
defineProps({
  page: { type: Number, required: true },
  pages: { type: Number, required: true },
  // The window of page numbers to offer, already slid — see usePaged.
  numbers: { type: Array, default: () => [] },
  // What is being paged, for the label a screen reader hears: "Documents, page 3 of 9"
  // beats "navigation" three times on one screen.
  label: { type: String, default: 'Pages' },
})

defineEmits(['go'])
</script>

<template>
  <nav v-if="pages > 1" class="pagination" :aria-label="label">
    <button
      class="pg" type="button" :disabled="page <= 1"
      aria-label="Previous page" @click="$emit('go', page - 1)"
    >
      ‹
    </button>
    <!-- aria-current, not a class: the recipe fills the current page from that attribute,
         so the state a screen reader reads and the state a sighted reader sees are one fact. -->
    <button
      v-for="n in numbers" :key="n" class="pg" type="button"
      :aria-current="n === page ? 'page' : null"
      :aria-label="`Page ${n}`" @click="$emit('go', n)"
    >
      {{ n }}
    </button>
    <button
      class="pg" type="button" :disabled="page >= pages"
      aria-label="Next page" @click="$emit('go', page + 1)"
    >
      ›
    </button>
  </nav>
</template>
