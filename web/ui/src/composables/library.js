/* ══ use/library.js — the documents, and the one form that edits them ═══════════
   The BA's library: what is in the knowledge base, and every change a person can make to it
   without leaving the app. The import panel adds files in bulk; this owns the rest —
   writing one document by hand, correcting one, renaming or moving it, and taking it out.

   Why it holds a form rather than just a list: after the inversion there is no file to open
   in an editor, so this screen is the only place a document can be written. That makes the
   form load-bearing, and it is why every field a document has is here rather than only the
   ones retrieval reads.

   Two rules the shapes follow:

   1. **The path is the identity.** Folder and name are two boxes because that is how a
      person thinks about where a document goes, and they are joined back into one path
      before it leaves — the server validates that path, and it is the same string a
      citation prints and a scope matches.
   2. **A save is one call.** Text, attributes and a move go together, so a BA fixing a typo
      while renaming cannot end up with one of the two applied.

   ── the form has to come to the person who opened it ──
   Opening it used to be `open.value = true` and nothing else. The form renders *below* the
   list, and the list sits below the import panel on a screen that also carries the queue —
   so on a phone, with nine documents indexed, pressing EDIT scrolled nothing, focused
   nothing, and looked exactly like a button that does not work. The BA's next move is to
   press it again, which reloads the document they had already loaded.

   `reveal` is the fix, and it is two separate failures: the form is scrolled to, and focus
   lands in its first field so a keyboard user is already typing. The form stays in the page
   rather than becoming a dialog — a BA writes a document while reading the list of what
   already exists, and that is what a modal takes away. Which is also why nothing here dims
   the list: the reason the form is not a modal is the reason it must not behave like one.
   What tells you where you are instead is the form's own head, which sticks.
   ═══════════════════════════════════════════════════════════════════════════ */
import { computed, nextTick, ref } from 'vue'
import { read, remove, write } from '../lib/upload.js'

/** The empty form: a new document, in no folder, of no stated kind. */
function blank() {
  return { path: '', folder: '', name: '', title: '', alias: '', kind: '', description: '', body: '' }
}

/**
 * @param {{ documents: () => Array<object>, toast: Function,
 *   onChanged: () => void, onLocked: (e: Error) => void }} deps
 *   `documents` is a getter, because the corpus object is replaced on every refresh and a
 *   held array would be one refresh behind. `onChanged` is what tells the shell to reload
 *   the corpus — a save changes what every other screen shows.
 * @returns {object} the list, the form, and the four things a BA can do
 */
export function useLibrary({ documents, toast, onChanged, onLocked }) {
  const query = ref('')
  const form = ref(blank())
  const editing = ref('') // the path being edited; "" means this is a new document
  const open = ref(false) // is the form showing at all
  const busy = ref(false)
  const error = ref('')
  // The path whose REMOVE has been pressed once. `drop` removes on the press it receives —
  // soft, so the text survives, but the document stops answering immediately — and the row it
  // sits in is a 40px target on a phone, beside EDIT. So the first press only arms, and the
  // button says SURE? until it acts. Arming another row moves it; a drop clears it.
  const armed = ref('')
  const arm = (path) => {
    armed.value = path
  }

  // The <form> element, so opening it can bring it to the person who pressed the button.
  // Bound by the template as `ref="formEl"` — the same shape App.vue uses for the prompt.
  const formEl = ref(null)

  /**
   * Scroll the form into view and put the caret in it.
   *
   * `nextTick` because the caller set `open` in this same tick and `v-if` has not rendered
   * the element yet — without it this reads `null` and silently does nothing, which is the
   * bug it exists to fix wearing a different hat.
   *
   * No `behavior` argument: the design system sets `html { scroll-behavior: smooth }` and
   * turns it off under `prefers-reduced-motion`, so saying "smooth" here would be a second
   * opinion that ignores the reader's. `preventScroll` on the focus stops the browser
   * jumping to the field before that scroll has run.
   */
  async function reveal() {
    await nextTick()
    const el = formEl.value
    if (!el)
      return
    el.scrollIntoView({ block: 'start' })
    el.querySelector('.input')?.focus({ preventScroll: true })
  }

  // Search across everything a person might remember about a document — its path, what it
  // is called, the other names it goes by, and what it is for. A library is searched by
  // half-remembered words, which is exactly what an alias is for.
  const shown = computed(() => {
    const q = query.value.trim().toLowerCase()
    const all = documents()
    if (!q)
      return all
    return all.filter(d => [d.path, d.title, d.alias, d.kind, d.description]
      .some(v => (v || '').toLowerCase().includes(q)))
  })

  const kinds = computed(() => [...new Set(documents().map(d => d.kind).filter(Boolean))].sort())
  const folders = computed(() => [...new Set(documents()
    .map(d => d.path.split('/').slice(0, -1).join('/'))
    .filter(Boolean))].sort())

  /** Start a new document. The folder carries over: a BA filing three specs stays in specs/. */
  function create(folder = '') {
    form.value = { ...blank(), folder }
    editing.value = ''
    error.value = ''
    open.value = true
    reveal()
  }

  /**
   * Load one document into the form. The body comes from the server rather than the list,
   * which deliberately does not carry bodies — a library of two hundred documents would
   * otherwise send two hundred of them to render a table.
   */
  async function edit(path) {
    busy.value = true
    error.value = ''
    try {
      const doc = await read(path)
      const cut = doc.path.lastIndexOf('/')
      form.value = {
        path: doc.path,
        folder: cut < 0 ? '' : doc.path.slice(0, cut),
        name: doc.path.slice(cut + 1),
        title: doc.title,
        alias: doc.alias,
        kind: doc.kind,
        description: doc.description,
        body: doc.body,
      }
      editing.value = path
      open.value = true
      reveal()
    }
    catch (e) {
      error.value = e.message
    }
    finally {
      busy.value = false
    }
  }

  /** Put the form away without saving. */
  function cancel() {
    open.value = false
    error.value = ''
    form.value = blank()
    editing.value = ''
  }

  /**
   * Save the form. One call carries the text, the attributes and — when the folder or name
   * changed — the new path, because a rename that half-applied would leave the old document
   * still answering.
   *
   * The name is checked here rather than only on the server: `.md` is the difference between
   * a document and a rejected upload, and a person who typed "pricing" deserves to be told
   * before the round trip.
   */
  async function save() {
    const f = form.value
    const name = f.name.trim()
    if (!name) {
      error.value = 'A file name is required — it is what a citation prints.'
      return
    }
    if (!/\.(?:md|markdown|txt)$/i.test(name)) {
      error.value = `"${name}" needs a .md, .markdown or .txt ending.`
      return
    }
    if (!f.body.trim()) {
      error.value = 'An empty document has nothing to retrieve.'
      return
    }

    const target = [f.folder.trim().replace(/^\/+|\/+$/g, ''), name].filter(Boolean).join('/')
    busy.value = true
    error.value = ''
    try {
      // For a new document the path in the URL *is* the target; for an edit the URL is what
      // exists today and `to` is where it should be, so the server moves the chunks with it
      // instead of leaving two documents that answer the same question.
      const res = await write(editing.value || target, {
        to: editing.value ? target : '',
        body: f.body,
        title: f.title.trim(),
        alias: f.alias.trim(),
        kind: f.kind.trim(),
        description: f.description.trim(),
      })
      toast(`Saved ${res.path} · ${res.chunks} sections indexed`, 'good')
      cancel()
      onChanged()
    }
    catch (e) {
      if (e.name === 'WrongPass') {
        onLocked(e)
        return
      }
      error.value = e.message
    }
    finally {
      busy.value = false
    }
  }

  /**
   * Take a document out of retrieval. Its text stays in the knowledge base with a removal
   * date, which is the only way back now that nothing on disk holds a second copy — so the
   * confirmation says "stops answering" rather than "deleted", because that is what happens.
   */
  async function drop(path) {
    busy.value = true
    error.value = ''
    armed.value = '' // it is acting now: the armed label must not survive the press
    try {
      const res = await remove(path)
      toast(`${res.path} no longer answers questions`, 'gold')
      if (editing.value === path)
        cancel()
      onChanged()
    }
    catch (e) {
      if (e.name === 'WrongPass') {
        onLocked(e)
        return
      }
      error.value = e.message
    }
    finally {
      busy.value = false
    }
  }

  return { query, shown, kinds, folders, form, formEl, editing, open, busy, error, armed, arm, create, edit, cancel, save, drop }
}
