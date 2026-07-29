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
  // The commit this server was built from, so "which version is deployed?" is answered by
  // looking at the app rather than by reaching the host. Empty for a build with no VCS
  // stamp, and then nothing renders — the same rule as every other field here.
  version: { type: String, default: '' },
  // The tag that commit was cut from, `v0.13.0`. When it is present the strip shows it
  // instead of the sha and the item becomes a button onto the notes — a reader asking
  // "what changed?" cannot get there from a hash. Empty means no tag was ever cut, and then
  // the commit is still shown, as plain text: the same rule as every other field here, where
  // absent renders nothing rather than a placeholder.
  release: { type: String, default: '' },
})
defineEmits(['showRelease'])
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
      <!-- Last, and the only field that is about the instance rather than the answer: a
           deploy is verified by reading it. No icon, like the two fields beside it — and
           deliberately not a `nes-icon name="branch"`, which this version of the library
           does not define, so it would have rendered an empty box with no warning.
           A trailing `+` means the host built from a dirty tree. -->
      <!-- With a tag: a real <button>, so it is reachable by keyboard and announced as
           something that does anything. `.sl-ver` only removes the button chrome — this is
           still an ambient strip, and a raised control in it would read as the primary
           action next to the prompt. -->
      <button
        v-if="release" type="button" class="sl-item sl-ver"
        :title="`Release ${release} — commit ${version}. Click for what changed.`"
        @click="$emit('showRelease')"
      >{{ release }}</button>
      <span
        v-else-if="version" class="sl-item"
        :title="`Deployed revision: commit ${version} (a trailing + means the tree was dirty)`"
      >@{{ version }}</span>
    </span>
  </div>
</template>
