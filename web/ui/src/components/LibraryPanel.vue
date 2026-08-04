<script setup>
/* ══ LibraryPanel.vue — the knowledge base, and the form that writes it ══════════
   A contract, like every other screen part: what comes in, what goes out, and a template
   over one composable.

     props   documents · writes    the library, and whether this instance may change it
     emit    changed               something moved: the shell reloads the corpus and the queue
     emit    locked(err)           the server refused the password — the gate handles it

   The layout is two blocks, and the order is the job: the list answers "what do we have?",
   and the form answers "make this one right". The form is below rather than in a dialog on
   purpose — a BA writes a document while reading the list of what already exists, and a modal
   is exactly what takes that away.

   Every control is a library recipe (.card · .field · .input · .textarea · .result ·
   .control-group · .btn · .badge), so this file contributes placement and nothing else. The
   list is `.result` rows because that is the recipe the ASK screen lists documents with, and
   one document should not have two appearances; `styles.css` says what the four `.lib-*`
   classes around them do.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { useLibrary } from '../composables/library.js'
import { usePaged } from '../composables/paged.js'
import { docTip, docTitle, folderCount, shortDate } from '../lib/library.js'
import Pager from './Pager.vue'

const props = defineProps({
  documents: { type: Array, default: () => [] },
  writes: Boolean, // a read-only instance shows the library and offers no way to change it
})

const emit = defineEmits(['changed', 'locked'])

// A getter, not props.documents: the corpus object is replaced on every refresh, and a held
// array would list the library as it was one save ago.
const {
  query,
  folder,
  shown,
  kinds,
  folders,
  form,
  formEl,
  editing,
  open,
  busy,
  error,
  armed,
  arm,
  create,
  edit,
  cancel,
  save,
  drop,
  selected,
  count,
  allShown,
  armedAll,
  armAll,
  toggle,
  toggleAll,
  clearSelection,
  dropSelected,
  progress,
  groups,
  collapsed,
  toggleFolder,
  groupAll,
  toggleGroup,
} = useLibrary({
  documents: () => props.documents,
  toast,
  onChanged: () => emit('changed'),
  onLocked: e => emit('locked', e),
})

// Pages the *folders* the Find field left, not the whole corpus and not the rows. A filter is
// the fast way to a known document and the pager is the way through an unknown one, so they
// have to compose — and `groups` is a getter here for the same reason usePaged asks for one:
// it is a fresh array on every keystroke, and the page must follow the search rather than the
// array it was built from.
//
// Groups rather than rows because a collapsed folder and a row pager disagree the moment
// anything is collapsed: the pager would go on counting rows nobody can see. Four is what fits
// a phone once the folders are open — the header, its documents, and the next header.
const { page, pages, numbers, slice: shownGroups, go } = usePaged(() => groups.value, 4)
</script>

<template>
  <section class="card">
    <div class="head">
      <span class="title">Library</span>
      <span class="badge">{{ documents.length }}</span>
      <div class="head-right">
        <button v-if="writes" class="btn sm" type="button" :disabled="busy" @click="create(form.folder)">
          NEW DOCUMENT
        </button>
      </div>
    </div>

    <!-- Searched by half-remembered words, which is what the alias field is for: the filter
         reads path, title, alias, kind and description together. -->
    <label class="field">
      <span class="label">Find</span>
      <input
        v-model="query" class="input" type="search"
        placeholder="name, folder, alias, kind, or what it is about"
      >
      <!-- "match", not "shown": the rows below are one page of these, and the two numbers
           stopped being the same thing the moment the list was paged. -->
      <span class="hint">{{ shown.length }} of {{ documents.length }} match</span>
    </label>

    <!-- Narrowing to a folder is a prefix on the path, not a word in the Find field: that
         field reads five fields as substrings, so "api" there also matches a document whose
         description mentions one. Selecting a folder is then "select all matches" — one idea
         rather than a second selection mechanism to keep in step with the first. -->
    <label v-if="folders.length" class="field">
      <span class="label">Folder</span>
      <select v-model="folder" class="select">
        <option value="">All folders</option>
        <option v-for="f in folders" :key="f" :value="f">{{ f }}</option>
      </select>
    </label>

    <!-- The bar is always here for someone who can write, rather than appearing once a row
         is ticked: "select all" that only exists after you have already selected something
         is a control nobody finds. The destructive half is what appears with the selection. -->
    <div v-if="writes && shown.length" class="lib-bulk">
      <label class="lib-all">
        <input
          class="checkbox" type="checkbox" :checked="allShown" :disabled="busy"
          :aria-label="`Select all ${shown.length} matching documents`" @change="toggleAll"
        >
        <!-- The count is the label, because the list is paged: a silent checkbox that reaches
             across pages is a trap, and one that says how far it reaches is not. -->
        <span>Select all {{ shown.length }}</span>
      </label>
      <template v-if="count">
        <span class="hint">{{ count }} selected</span>
        <button class="btn ghost xs" type="button" :disabled="busy" @click="clearSelection">
          CLEAR
        </button>
        <button
          class="btn xs" type="button" :disabled="busy" :data-accent="armedAll ? 'crit' : null"
          :aria-label="armedAll ? `Confirm: remove ${count} documents` : `Remove ${count} documents`"
          @click="armedAll ? dropSelected() : armAll()"
        >
          <template v-if="progress">REMOVING {{ progress.done }}/{{ progress.total }}</template>
          <template v-else-if="armedAll">SURE? REMOVE {{ count }}</template>
          <template v-else>REMOVE {{ count }}</template>
        </button>
      </template>
    </div>

    <div v-if="!documents.length" class="empty">
      <span class="icon">◈</span>
      <span class="title">Nothing indexed yet</span>
      <p>Import files above, or write the first document here.</p>
    </div>

    <!-- One document, one `.result` — the same row the ASK screen's document menu is built
         from, so a document looks the same wherever it is listed. Its two lines ellipsise, so
         a 90-character path can never re-shape a row; the alias and the description are in
         the row's `title` because their job is being *searched* (the Find field reads them),
         not being scanned. -->
    <div v-else class="lib-rows">
      <!-- The folder, as a header that is three controls: tick the whole group, fold it away,
           and narrow the screen to it. The counts are the group's own totals, not the part
           visible — which is only true because the pager pages groups and never splits one. -->
      <template v-for="g in shownGroups" :key="g.folder">
        <div class="lib-group">
          <input
            v-if="writes" class="checkbox" type="checkbox"
            :checked="groupAll(g)" :disabled="busy"
            :aria-label="`Select every document in ${g.folder || 'the top level'}`"
            @change="toggleGroup(g)"
          >
          <button
            class="lib-fold" type="button"
            :aria-expanded="!collapsed.has(g.folder)"
            :aria-label="`${collapsed.has(g.folder) ? 'Expand' : 'Collapse'} ${g.folder || 'the top level'}`"
            @click="toggleFolder(g.folder)"
          >
            {{ collapsed.has(g.folder) ? '▸' : '▾' }}
          </button>
          <!-- The name narrows the screen to that folder, which is the same string the ASK
               screen scopes a question to. One fact, two verbs. -->
          <button
            class="lib-dir" type="button" :disabled="!g.folder"
            :aria-label="g.folder ? `Show only ${g.folder}` : 'Documents at the top level'"
            @click="folder = g.folder"
          >
            {{ g.folder ? `${g.folder}/` : 'top level' }}
          </button>
          <span class="hint">{{ folderCount(g.docs.length, g.chunks) }}</span>
        </div>
        <div
          v-for="d in (collapsed.has(g.folder) ? [] : g.docs)" :key="d.path"
          class="lib-row"
        >
          <!-- Outside `.result` rather than inside it: that recipe is a row of its own with an
             icon, two lines and its badges, and a control dropped into it inherits the row's
             pointer behaviour. The tick belongs to the list, not to the document. -->
          <input
            v-if="writes" class="checkbox" type="checkbox"
            :checked="selected.has(d.path)" :disabled="busy"
            :aria-label="`Select ${d.path}`" @change="toggle(d.path)"
          >
          <div class="result" :title="docTip(d)">
            <nes-icon class="result-icon" name="file" />
            <span class="result-body">
              <!-- `docTitle` takes the document, not its path: given a string it read
                 `.title` off it, found nothing, and returned "" — so a document with no title
                 of its own had an empty row. It already falls back to the file name. -->
              <span class="result-title">{{ docTitle(d) }}</span>
              <span class="result-path">{{ d.path }}</span>
            </span>
            <!-- Plain .badge, not .clear: in this design system `clear` is the *good/green*
               status fill, so a kind and a section count were claiming a pass state — and on
               the count it also outscored the data-accent beside it. -->
            <span v-if="d.kind" class="badge">{{ d.kind }}</span>
            <span
              class="badge" :data-accent="d.approved ? 'good' : 'blue'"
              :title="`${d.chunks} retrievable sections, ${d.approved} confirmed by a BA`"
            >{{ d.chunks }}</span>
            <span class="result-hint">{{ shortDate(d.updated_at) }}</span>
          </div>
          <!-- Icons, so both actions fit on the row instead of adding a second line to every
             document — `edit` and `trash` are both in the pinned icons.d.ts, which is the only
             way to know: a name the release does not have renders an empty box and says
             nothing. The label a screen reader gets is on the button. -->
          <div v-if="writes" class="lib-acts">
            <button
              class="btn ghost xs icon" type="button" :disabled="busy"
              aria-label="Edit this document" @click="edit(d.path)"
            >
              <nes-icon name="edit" />
            </button>
            <!-- Two presses, and the label is what says so. `drop` removes on the press it
               receives, so the arming has to be in front of it or a mis-tap on a phone takes
               a document out of the knowledge base — this button used to carry the library's
               `.perm` class, which is a confirmation *block* recipe (a bordered card with its
               own actions), not a modifier: it made the button a full-width panel and confirmed
               nothing. -->
            <button
              class="btn xs" :class="armed === d.path ? '' : 'ghost icon'" type="button"
              :disabled="busy" :data-accent="armed === d.path ? 'crit' : null"
              :aria-label="armed === d.path ? 'Confirm: remove this document' : 'Remove this document'"
              @click="armed === d.path ? drop(d.path) : arm(d.path)"
            >
              <nes-icon v-if="armed !== d.path" name="trash" />
              <template v-else>SURE?</template>
            </button>
          </div>
        </div>
      </template>
    </div>

    <Pager :page="page" :pages="pages" :numbers="numbers" label="Library folders" @go="go" />

    <!-- ══ the form ═══════════════════════════════════════════════════════════
         Six fields and the text. Everything above the body is what makes a document
         findable by a person six months later; the body is what answers questions.
         ═══════════════════════════════════════════════════════════════════ -->
    <!-- `ref="formEl"` is how the composable brings this to whoever pressed EDIT: the panel
         sits below the import card on a screen that also carries the queue, so opening a form
         at the bottom of it used to look exactly like a button that does nothing.
         Escape cancels. The listener is on the form rather than the window because focus is
         already inside it — a global key handler would also fire while a BA is reading the
         queue, which is a different screen's business. -->
    <form
      v-if="open" ref="formEl" class="card doc-form" data-accent="blue"
      @submit.prevent="save" @keydown.esc="cancel"
    >
      <!-- Sticky, because the six fields and a 14-row textarea are taller than a phone: the
           one thing a person must not lose track of while scrolling a form is which document
           they are changing. The icon says which of the two jobs this is — writing a new
           document and correcting an existing one look identical otherwise, and only one of
           them overwrites something.

           An icon rather than the EDITING/NEW eyebrow it was: the word and the path are the
           same fact at two sizes, and on a 390px head the label took a third of the row from
           the one thing that has to stay readable. `edit` and `plus` are the two verbs the
           library already draws, and `title` carries the word for a pointer. -->
      <div class="head doc-form-head">
        <nes-icon
          :name="editing ? 'edit' : 'plus'"
          :title="editing ? 'Editing an existing document' : 'Writing a new document'"
        />
        <span class="title">{{ editing || 'New document' }}</span>
        <button
          class="btn ghost xs icon" type="button" :disabled="busy"
          aria-label="Close this form without saving" @click="cancel"
        >
          <nes-icon name="close" />
        </button>
      </div>

      <div class="control-group row">
        <label class="field">
          <span class="label">Folder</span>
          <input
            v-model="form.folder" class="input" list="lib-folders"
            placeholder="business/pricing"
          >
          <span class="hint">What a reader scopes a question to. Blank means the top level.</span>
        </label>
        <label class="field">
          <span class="label">File name</span>
          <input v-model="form.name" class="input" placeholder="2026.md" required>
          <span class="hint">Ends in .md, .markdown or .txt — it is what a citation prints.</span>
        </label>
      </div>
      <datalist id="lib-folders">
        <option v-for="f in folders" :key="f" :value="f" />
      </datalist>

      <div class="control-group row">
        <label class="field">
          <span class="label">Title</span>
          <input v-model="form.title" class="input" placeholder="2026 pricing">
          <span class="hint">Shown on screen. The file name is used when this is blank.</span>
        </label>
        <label class="field">
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

      <div class="control-group row">
        <button class="btn" type="submit" :disabled="busy">
          {{ busy ? 'SAVING…' : 'SAVE' }}
        </button>
        <button class="btn ghost" type="button" :disabled="busy" @click="cancel">CANCEL</button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.head-right {
  width: 100%;
  display: flex;
  justify-content: end;
}
</style>
