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
defineEmits(['showRelease', 'showSettings'])
</script>

<template>
  <div class="statusline" :data-state="line.state" role="status" aria-live="polite">
    <span class="sl-state">{{ line.label }}</span>
    <!-- The model is the one field in this strip that is also a *choice*, so it is the door to
         the panel that holds it rather than a second picker: one place to change it, one place
         to read it. `.sl-ver` already removes the button chrome for the version item — same
         class, same reason, and nothing in an ambient strip should look like the Send button. -->
    <button
      v-if="model" class="sl-item sl-ver" type="button"
      :title="`Chat model: ${model} — click for settings`" @click="$emit('showSettings')"
    >
      <nes-icon name="cpu" aria-hidden="true" />{{ model }}
    </button>
    <span
      v-if="line.tokens" class="sl-item"
      title="Prompt + completion tokens the provider reported"
    >
      <nes-icon name="layers" aria-hidden="true" />{{ line.tokens }}
    </span>
    <span v-if="line.refs" class="sl-item" title="Sources this answer cited">
      <nes-icon name="link" aria-hidden="true" />{{ line.refs }}
    </span>
    <!-- Memory, as kept/offered. Absent until there is a thread, because a figure about
         nothing is noise — and present the moment one turn was trimmed, because a silent trim
         is indistinguishable from an assistant that forgot.
         `chat` and not `history`: 0.13.0 ships no `history`, and a name the release does not
         have renders an empty box and says nothing — checked in icons.d.ts, which is the only
         place that answers it. -->
    <span
      v-if="line.recall" class="sl-item"
      title="Earlier turns this answer read, of those offered — trimmed to the model's context window"
    >
      <nes-icon name="chat" aria-hidden="true" />{{ line.recall }}
    </span>
    <!-- Grounding, as sections-read/sections-weighed. Absent when the two agree, which is
         every instance that never configured a window: it reads all of TOP_K and the pair
         reports nothing. `database` is in 0.15.0's icons.d.ts; the corpus is what it counts. -->
    <span
      v-if="line.sections" class="sl-item"
      title="Sections of the corpus this answer was built from, of those retrieval weighed"
    >
      <nes-icon name="database" aria-hidden="true" />{{ line.sections }}
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
