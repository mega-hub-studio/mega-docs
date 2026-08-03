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
import { usePaged } from '../composables/paged.js'
import Pager from './Pager.vue'

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
  pending,
  removing,
  askRemove,
  cancelRemove,
  confirmRemove,
} = useImporter({
  documents: () => props.documents,
  toast,
  onIndexed: () => emit('indexed'),
  onLocked: e => emit('locked', e),
})

// The remove drawer lists the whole corpus. Closed by default is what kept that survivable;
// a pager is what makes it usable once it is open, because the way to a document in a list of
// two hundred was scrolling past a hundred and ninety-nine REMOVE buttons.
const { page, pages, numbers, slice: removable, go } = usePaged(() => props.documents, 10)
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
      Written into the knowledge base and indexed immediately. The same file name updates
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
         stuck.

         --fill goes on the <i>, which is the form the recipe's own docs show. 0.15.0 made
         that the only form that worked — it registered the property `inherits: false`, so a
         value on the container resolved to the registered initial and the bar was empty at
         every count (measured: 0px against 1031.89px at the same 66.66%). 0.16.0 accepts
         both, reported from here; the child stays because it is what the docs document. -->
    <div v-if="importing" class="importing" role="status" aria-live="polite">
      <p class="hint">
        <span class="spinner sm" aria-hidden="true" />
        Indexing {{ progress.done + 1 > progress.total ? progress.total : progress.done + 1 }}
        of {{ progress.total }} — each file is embedded before it is searchable.
      </p>
      <!-- Only when there is more than one file, because progress is counted per *file*: the
           request carries the embedding, so nothing is known about a file until it lands. One
           file therefore has no intermediate state at all — the bar would sit at 0% for the
           whole import and reach 100% in the same tick that unmounts it, which is the "empty
           bar" this whole block was reported for. A bar stuck at zero says "not started" while
           the work is running, and that is the same lie as a bar that invents its position,
           told from the other end. The spinner and the sentence are the honest signal there. -->
      <div
        v-if="progress.total > 1"
        class="pbar" role="progressbar"
        :aria-valuenow="progress.done" aria-valuemin="0" :aria-valuemax="progress.total"
      >
        <i :style="{ '--fill': `${progress.done / progress.total * 100}%` }" />
      </div>
    </div>

    <!-- Per file, both outcomes — but not with equal weight, which is what one flat list gave
         them. A batch where seven landed and one was a PDF is the normal case and the eighth
         is the only line anyone has to act on; importing a folder of 300 buried it under 300
         INDEXED rows nobody reads. So the failures are the list, and the successes are a count
         with the detail behind it.

         The <details> is *not* a pager: a per-file report is read once and thrown away, so the
         thing it needs is to stop being in the way, not a way to walk it. -->
    <template v-if="imported">
      <dl v-if="imported.failed.length" class="datalist">
        <template v-for="f in imported.failed" :key="f.name">
          <dt><span class="badge crit">SKIPPED</span></dt>
          <dd><code>{{ f.name }}</code> — {{ f.error }}</dd>
        </template>
      </dl>
      <details v-if="imported.uploaded.length" class="corpus">
        <summary>
          <span class="eyebrow">
            {{ imported.uploaded.length }} indexed — {{ imported.chunks }} sections
          </span>
        </summary>
        <dl class="datalist">
          <template v-for="d in imported.uploaded" :key="d.path">
            <dt><span class="badge clear">INDEXED</span></dt>
            <dd><code>{{ d.path }}</code> — {{ d.chunks }} sections</dd>
          </template>
        </dl>
      </details>
    </template>

    <p class="hint">PDF or DOCX? Convert first!</p>

    <!-- Removal belongs on this card rather than a screen of its own: it is the same
         password, the same corpus, and the same person's job. Collapsed, because a BA opens
         this card to *add* something far more often than to take one away — and an expanded
         list of every indexed path would bury the import controls above it. -->
    <details v-if="documents.length" class="corpus">
      <summary>
        <span class="eyebrow">Remove a document — {{ documents.length }} indexed</span>
      </summary>
      <dl class="datalist">
        <template v-for="d in removable" :key="d.path">
          <dt><code>{{ d.path }}</code></dt>
          <dd>
            <button
              class="btn ghost sm" data-accent="crit"
              :disabled="!!removing || !!pending" @click="askRemove(d.path)"
            >
              {{ removing === d.path ? 'REMOVING…' : 'REMOVE' }}
            </button>
          </dd>
        </template>
      </dl>
      <Pager :page="page" :pages="pages" :numbers="numbers" label="Document pages" @go="go" />
    </details>

    <!-- .perm is the library's own gate: one request, one decision, the target shown
         verbatim. Exactly right here — the paths in the list above differ by one word, so
         the thing that must be unambiguous is *which* document, not the wording of the
         warning. -->
    <div v-if="pending" class="perm" role="alertdialog" aria-live="polite">
      <span class="perm-kind">Remove</span>
      <code class="perm-target">{{ pending }}</code>
      <p class="perm-why">
        Answers that cite this stop citing it, and a BA-confirmed answer removed here is gone
        from the queue's history too. Its text survives: removal sets a date on the row rather
        than destroying it, so whoever has the database can bring it back.
      </p>
      <div class="perm-actions">
        <button class="btn" data-accent="crit" @click="confirmRemove">
          REMOVE IT
        </button>
        <button class="btn ghost" @click="cancelRemove">
          CANCEL
        </button>
      </div>
    </div>
  </section>
</template>
