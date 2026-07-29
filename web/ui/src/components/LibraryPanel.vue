<script setup>
/* ══ LibraryPanel.vue — the knowledge base, and the form that writes it ══════════
   A contract, like every other screen part: what comes in, what goes out, and a template
   over one composable.

     props   documents · writes    the library, and whether this instance may change it
     emit    changed               something moved: the shell reloads the corpus and the queue
     emit    locked(err)           the server refused the password — the gate handles it

   The layout is two blocks, and the order is the job: the table answers "what do we have?",
   and the form answers "make this one right". The form is below rather than in a dialog on
   purpose — a BA writes a document while reading the list of what already exists, and a modal
   is exactly what takes that away.

   Every control is a library recipe (.card · .field · .input · .textarea · .datalist ·
   .segment · .btn · .badge), so this file contributes placement and nothing else.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { useLibrary } from '../composables/library.js'
import { docTitle, shortDate } from '../lib/library.js'

const props = defineProps({
  documents: { type: Array, default: () => [] },
  writes: Boolean, // a read-only instance shows the library and offers no way to change it
})

const emit = defineEmits(['changed', 'locked'])

// A getter, not props.documents: the corpus object is replaced on every refresh, and a held
// array would list the library as it was one save ago.
const {
  query,
  shown,
  kinds,
  folders,
  form,
  editing,
  open,
  busy,
  error,
  create,
  edit,
  cancel,
  save,
  drop,
} = useLibrary({
  documents: () => props.documents,
  toast,
  onChanged: () => emit('changed'),
  onLocked: e => emit('locked', e),
})
</script>

<template>
  <section class="card lib">
    <div class="head">
      <span class="title">Library</span>
      <span class="badge clear">{{ documents.length }}</span>
      <button
        v-if="writes" class="btn sm" type="button" :disabled="busy"
        @click="create(form.folder)"
      >
        NEW DOCUMENT
      </button>
    </div>

    <!-- Searched by half-remembered words, which is what the alias field is for: the filter
         reads path, title, alias, kind and description together. -->
    <label class="field">
      <span class="label">Find</span>
      <input
        v-model="query" class="input" type="search"
        placeholder="name, folder, alias, kind, or what it is about"
      >
      <span class="hint">{{ shown.length }} of {{ documents.length }} shown</span>
    </label>

    <div v-if="!documents.length" class="empty">
      <span class="icon">◈</span>
      <span class="title">Nothing indexed yet</span>
      <p>Import files above, or write the first document here.</p>
    </div>

    <!-- .datalist is the library's row list. The columns are what a BA scans for: what it is
         called, where it lives, what kind it is, and whether retrieval can use it. -->
    <div v-else class="datalist">
      <div v-for="d in shown" :key="d.path" class="row">
        <div class="grow">
          <b>{{ d.title || docTitle(d.path) }}</b>
          <span v-if="d.alias" class="hint"> · {{ d.alias }}</span>
          <div class="hint">
            <code>{{ d.path }}</code>
            <template v-if="d.description"> — {{ d.description }}</template>
          </div>
        </div>
        <span v-if="d.kind" class="badge clear">{{ d.kind }}</span>
        <span
          class="badge clear" :data-accent="d.approved ? 'good' : 'blue'"
          :title="`${d.chunks} retrievable sections, ${d.approved} confirmed by a BA`"
        >{{ d.chunks }}</span>
        <span class="hint">{{ shortDate(d.updated_at) }}</span>
        <button
          v-if="writes" class="btn ghost sm" type="button" :disabled="busy"
          @click="edit(d.path)"
        >
          EDIT
        </button>
        <!-- .perm is the library's destructive confirmation: the second press is the one
             that acts, so a mis-tap costs nothing. -->
        <button
          v-if="writes" class="btn ghost sm perm" type="button" :disabled="busy"
          @click="drop(d.path)"
        >
          REMOVE
        </button>
      </div>
    </div>

    <!-- ══ the form ═══════════════════════════════════════════════════════════
         Six fields and the text. Everything above the body is what makes a document
         findable by a person six months later; the body is what answers questions.
         ═══════════════════════════════════════════════════════════════════ -->
    <form v-if="open" class="card doc-form" data-accent="blue" @submit.prevent="save">
      <div class="head">
        <span class="title">{{ editing || 'New document' }}</span>
      </div>

      <div class="row">
        <label class="field grow">
          <span class="label">Folder</span>
          <input
            v-model="form.folder" class="input" list="lib-folders"
            placeholder="business/pricing"
          >
          <span class="hint">What a reader scopes a question to. Blank means the top level.</span>
        </label>
        <label class="field grow">
          <span class="label">File name</span>
          <input v-model="form.name" class="input" placeholder="2026.md" required>
          <span class="hint">Ends in .md, .markdown or .txt — it is what a citation prints.</span>
        </label>
      </div>
      <datalist id="lib-folders">
        <option v-for="f in folders" :key="f" :value="f" />
      </datalist>

      <div class="row">
        <label class="field grow">
          <span class="label">Title</span>
          <input v-model="form.title" class="input" placeholder="2026 pricing">
          <span class="hint">Shown on screen. The file name is used when this is blank.</span>
        </label>
        <label class="field grow">
          <span class="label">Kind</span>
          <input v-model="form.kind" class="input" list="lib-kinds" placeholder="spec · policy · runbook">
          <span class="hint">Your own vocabulary — whatever you sort by.</span>
        </label>
      </div>
      <datalist id="lib-kinds">
        <option v-for="k in kinds" :key="k" :value="k" />
      </datalist>

      <label class="field">
        <span class="label">Alias</span>
        <input v-model="form.alias" class="input" placeholder="rate card, giá 2026">
        <span class="hint">The other names people ask for it by. Searched with everything else.</span>
      </label>

      <label class="field">
        <span class="label">Description</span>
        <input v-model="form.description" class="input" placeholder="what is in it, and when to reach for it">
      </label>

      <label class="field">
        <span class="label">Document</span>
        <textarea
          v-model="form.body" class="textarea" rows="14"
          placeholder="# Heading&#10;&#10;Markdown. Headings become the breadcrumb a citation shows."
          required
        />
        <span class="hint">Saving re-indexes it, so the next answer uses this text.</span>
      </label>

      <div v-if="error" class="callout crit" role="alert">{{ error }}</div>

      <div class="row">
        <button class="btn" type="submit" :disabled="busy">
          {{ busy ? 'SAVING…' : 'SAVE' }}
        </button>
        <button class="btn ghost" type="button" :disabled="busy" @click="cancel">CANCEL</button>
      </div>
    </form>
  </section>
</template>
