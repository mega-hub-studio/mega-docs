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

defineProps({
  clarify: { type: Object, required: true },
})

defineEmits(['refine'])

// Read once on submit. Nothing ticked composes to "", and ask() already returns on a blank
// question — so an empty submit is a silent no-op rather than a case handled here.
const picked = event => composeClarify(new FormData(event.target))
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
        <!-- The same `.cite` recipe the prose above renders a marker with, so a citation looks
             like a citation wherever it appears — this row used to print "[2]" as characters
             beside a card full of chips.

             A <span>, not the <a> prose uses: the row is a <label>, so an anchor inside it
             would tick the checkbox on the way to the source. The number is provenance here —
             which section backs this reading — and the source list is already on screen a few
             blocks below, under the card. -->
        <span v-for="n in opt.cites" :key="n" class="cite">{{ n }}</span>
        <!-- Ticked already, so this only says why — the fastest path is one tap on ASK THIS,
             and the reader can still disagree with the pick.

             A star, not the word. `.check` is `inline-flex` with no wrap and the option text is
             its only shrinkable child, so a 90px uppercase chip took that width out of the
             sentence it was labelling: on a phone the question broke into a three-word column
             beside a badge nobody needs to read twice. The icon is ~14px and says the same
             thing in the space of one character. `role`/`aria-label` because the library
             renders the svg `aria-hidden` — without them the mark is invisible to a screen
             reader, and `title` alone is not reliably announced. -->
        <nes-icon
          v-if="opt.recommended" name="star"
          role="img" aria-label="Recommended" title="Recommended"
        />
      </label>
    </fieldset>
    <button class="btn xs" type="submit">ASK THIS</button>
  </form>
</template>
