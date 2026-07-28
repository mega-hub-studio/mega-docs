<script setup>
/* ══ ImportPanel.vue — the other way a document enters the corpus ════════════════
   Behind the same password as a confirm: this app has no accounts, so an open import
   would let anyone who reaches the port rewrite what everyone else reads. The parent only
   renders this once writes are unlocked; what it needs back is the two facts it cannot
   see from here — a file landed (refresh the corpus) and the password was refused.

   A drop zone *and* a button, because a drop is invisible to anyone who doesn't already
   know it works — and impossible from a phone.

   The composable owns every branch (which files are usable, how far the batch got, what
   failed and why). This file owns how that reads.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { useImporter } from '../composables/importer.js'

const props = defineProps({
  documents: { type: Array, default: () => [] },
})

const emit = defineEmits(['indexed', 'locked'])

const {
  accept,
  importDir,
  importing,
  progress,
  dragging,
  imported,
  folders,
  importDocs,
  pickDocs,
} = useImporter({
  documents: () => props.documents,
  toast,
  onIndexed: () => emit('indexed'),
  onLocked: e => emit('locked', e),
})
</script>

<template>
  <section
    class="card drop" data-accent="blue" :data-over="String(dragging)"
    @dragenter.prevent="dragging = true"
    @dragover.prevent="dragging = true"
    @dragleave.prevent="dragging = false"
    @drop.prevent="importDocs($event.dataTransfer.files)"
  >
    <div class="head">
      <span class="title">Import documents</span>
      <span class="badge todo">{{ accept }}</span>
    </div>
    <p>
      Written into <code>docs/</code> and indexed immediately. The same file name updates
      that document in place, so re-importing a corrected version replaces it instead of
      adding a second copy.
    </p>

    <!-- The folder is the scope. A reader later browses this tree and asks a question
         against one branch, so choosing it at import time is not filing — it is what
         makes the answer findable. Existing folders are offered, because a fourth
         spelling of "engineering" is a fourth scope. -->
    <label class="field">
      <span class="label">Folder</span>
      <input
        v-model="importDir" class="input" list="corpus-folders" placeholder="business/pricing"
        autocapitalize="off" autocorrect="off" spellcheck="false"
      >
      <datalist id="corpus-folders">
        <option v-for="f in folders" :key="f" :value="f" />
      </datalist>
      <span class="hint">Empty = the top level. Picking a whole folder keeps its own structure.</span>
    </label>

    <div class="control-group">
      <label class="btn">
        <input type="file" :accept="accept" multiple hidden :disabled="importing" @change="pickDocs">
        {{ importing ? 'IMPORTING…' : 'CHOOSE FILES' }}
      </label>
      <label class="btn ghost">
        <input type="file" webkitdirectory multiple hidden :disabled="importing" @change="pickDocs">
        CHOOSE A FOLDER
      </label>
    </div>

    <!-- Progress, and only real progress. One request per file is what makes the number
         honest — a single POST could only be animated by guessing, and a bar that invents
         its position says "nearly done" while the last file has not started. .pbar is the
         design system's determinate bar (--fill drives its <i>); the spinner covers the
         moment before the first file returns, when the bar would sit at 0 and read as
         stuck. -->
    <div v-if="importing" class="importing" role="status" aria-live="polite">
      <p class="hint">
        <span class="spinner sm" aria-hidden="true" />
        Indexing {{ progress.done + 1 > progress.total ? progress.total : progress.done + 1 }}
        of {{ progress.total }} — each file is embedded before it is searchable.
      </p>
      <div
        class="pbar" role="progressbar"
        :style="{ '--fill': `${progress.total ? progress.done / progress.total * 100 : 0}%` }"
        :aria-valuenow="progress.done" aria-valuemin="0" :aria-valuemax="progress.total"
      >
        <i />
      </div>
    </div>

    <!-- Per file, both outcomes in one list: a batch where seven landed and one was a PDF
         is the normal case, and the user needs to see which. -->
    <dl v-if="imported" class="datalist">
      <template v-for="d in imported.uploaded" :key="d.path">
        <dt><span class="badge clear">INDEXED</span></dt>
        <dd><code>{{ d.path }}</code> — {{ d.chunks }} sections</dd>
      </template>
      <template v-for="f in imported.failed" :key="f.name">
        <dt><span class="badge crit">SKIPPED</span></dt>
        <dd><code>{{ f.name }}</code> — {{ f.error }}</dd>
      </template>
    </dl>

    <p class="hint">PDF or DOCX? Convert first — <code>markitdown spec.pdf &gt; spec.md</code>.</p>
  </section>
</template>
