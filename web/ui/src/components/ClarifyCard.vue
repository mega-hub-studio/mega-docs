<script setup>
/* ══ ClarifyCard.vue — the reply that asks instead of guessing ═══════════════════
   A contract, like every other part: what comes in, what goes out, one template.

     props   clarify      the block lib/answer.js read out of the reply
     emit    refine(q)    the question those picks are worth asking — the shell asks it

   Two kinds arrive here and the difference is only the tone. A [!QUESTION] card is the whole
   reply: the documents cover the question two ways, so answering one of them silently would
   be a wrong answer delivered confidently. A [!NEXT] card sits under a finished answer and
   offers the questions that same retrieval can still answer.

   It is a real <form> rather than a set of bound refs because the native checkboxes already
   hold which options are ticked — a ref mirroring them would be a second copy of the state
   the DOM is keeping. Every control is a library recipe (.callout · .control-group · .check ·
   .checkbox · .badge · .btn), so this file contributes placement and nothing else.
   ═══════════════════════════════════════════════════════════════════════════ */
import { composeClarify } from '../lib/answer.js'

const props = defineProps({
  clarify: { type: Object, required: true },
})

defineEmits(['refine'])

// Read once on submit. Nothing ticked composes to "", and ask() already returns on a blank
// question — so an empty submit is a silent no-op rather than a case handled here.
const picked = event => composeClarify(props.clarify, new FormData(event.target))
</script>

<template>
  <form
    class="callout clarify"
    :class="clarify.kind === 'QUESTION' ? 'quest' : 'info'"
    @submit.prevent="$emit('refine', picked($event))"
  >
    <!-- fieldset/legend, not a div and a bold line: the prompt is the group's name, so a
         screen reader should read it with every option rather than once above them. -->
    <fieldset class="control-group">
      <legend>{{ clarify.prompt }}</legend>
      <label v-for="(opt, i) in clarify.options" :key="i" class="check">
        <!-- Checkboxes, not radios: two readings of one question are sometimes both wanted
             ("how do these differ?"), and that is a question the corpus can answer. -->
        <input
          class="checkbox"
          type="checkbox"
          name="reading"
          :value="opt.text"
          :checked="opt.recommended"
        >
        <span>{{ opt.text }}</span>
        <!-- Ticked already, so the badge only says why — the fastest path is one tap on
             ASK THIS, and the reader can still disagree with the pick. -->
        <span v-if="opt.recommended" class="badge">RECOMMENDED</span>
      </label>
    </fieldset>
    <button class="btn xs" type="submit">ASK THIS</button>
  </form>
</template>
