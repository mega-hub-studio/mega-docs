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
import { computed, nextTick, ref, watch } from 'vue'
import { KINDS, read, remove, titleFrom, write } from '../lib/upload.js'

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
  /* ── clearing out several documents at once ──────────────────────────────────
     A library nobody prunes answers from dead documents, and pruning it was one row at a
     time behind a two-press arm — forty presses to retire twenty specs, across a paged
     list. So the rows carry a checkbox and the bar acts on all of them.

     Selecting *a folder* is not a second mechanism: `folder` narrows the list, and "select
     all matches" then means that folder. One idea instead of two, and it is the idea a
     reader already has from the Find field. What it is NOT is the Find text — that matches
     a substring across five fields, so typing "api" also picks up a document whose
     description mentions an API. A folder has to be a prefix on the path, which is the
     document's identity, or the selection quietly includes things nobody looked at. */
  const folder = ref('')
  const selected = ref(new Set())
  const armedAll = ref(false)
  const progress = ref(null) // {done, total} while a bulk removal runs

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

  const folders = computed(() => [...new Set(documents()
    .map(d => d.path.split('/').slice(0, -1).join('/'))
    .filter(Boolean))].sort())

  /* A folder that no longer exists cannot stay picked, and removing every document in one is
     exactly how you get there — measured: emptying booking/ left ten rows gone, a list
     reading "0 of 4 match", and a dropdown showing nothing selected, with no way back that
     looked like one. Reading through this computed rather than resetting `folder` from a
     watcher keeps it one fact: the filter *is* whichever of the folders still exists. */
  const pickedFolder = computed({
    get: () => (folders.value.includes(folder.value) ? folder.value : ''),
    set: (v) => {
      folder.value = v || ''
    },
  })

  // Search across everything a person might remember about a document — its path, what it
  // is called, the other names it goes by, and what it is for. A library is searched by
  // half-remembered words, which is exactly what an alias is for.
  const shown = computed(() => {
    const q = query.value.trim().toLowerCase()
    const dir = pickedFolder.value ? `${pickedFolder.value}/` : ''
    let all = documents()
    // The folder carries its own separator, or "qa" would also match "qawhatever/x.md" —
    // the same rule the engine's own scope filter follows, for the same reason.
    if (dir)
      all = all.filter(d => (d.path || '').startsWith(dir))
    if (!q)
      return all
    return all.filter(d => [d.path, d.title, d.alias, d.kind, d.description]
      .some(v => (v || '').toLowerCase().includes(q)))
  })

  // The selection, intersected with what is actually in the library. A path removed in
  // another tab would otherwise keep its place in the count, and the bar would offer to
  // remove a document that is already gone.
  const picked = computed(() => documents().filter(d => selected.value.has(d.path)))
  const count = computed(() => picked.value.length)
  const allShown = computed(() => shown.value.length > 0
    && shown.value.every(d => selected.value.has(d.path)))

  const arm = (path) => {
    armed.value = path
    armedAll.value = false // two SURE? on one screen is two questions and one answer
  }

  const armAll = () => {
    armedAll.value = true
    armed.value = ''
  }

  function toggle(path) {
    if (!selected.value.delete(path))
      selected.value.add(path)
    armedAll.value = false // the set changed, so the number on the armed button is stale
  }

  /** Every match, not every visible row — the list is paged, and the label says which. */
  function toggleAll() {
    const all = allShown.value
    for (const d of shown.value) {
      if (all)
        selected.value.delete(d.path)
      else selected.value.add(d.path)
    }
    armedAll.value = false
  }

  function clearSelection() {
    selected.value.clear()
    armedAll.value = false
  }

  // The seeded list and whatever the corpus has taught it, as one set. Seeded alone would
  // hide a team's own vocabulary; derived alone cannot start, because an empty corpus offers
  // nothing and the first author invents a spelling everyone else then near-misses.
  const kinds = computed(() => [...new Set([
    ...KINDS,
    ...documents().map(d => d.kind).filter(Boolean),
  ])].sort())

  /* ── what the path already said ──────────────────────────────────────────────
     A BA filing into `runbook/` has told us the kind, and naming the file `refund-policy.md`
     has told us the title — so asking for both again is asking someone to repeat themselves
     into a form. Only those two: an alias is the half-remembered word someone else will
     search by and a description is a claim about the document, and a machine-written claim is
     a fact nobody vouched for, which is the one thing this library is built not to hold.

     Empty fields only, so nothing a person typed is ever overwritten — and new documents
     only. On an edit a blank title is a choice that was already made, not a gap. */
  watch(() => [form.value.folder, form.value.name], () => {
    if (editing.value)
      return
    const f = form.value
    const leaf = (f.folder || '').split('/').findLast(Boolean)?.toLowerCase() ?? ''
    if (!f.kind && KINDS.includes(leaf))
      f.kind = leaf
    if (!f.title && f.name)
      f.title = titleFrom(f.name)
  })

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
      // An options object, not `'good'`: the second argument is destructured, so a bare
      // string reads no `accent` off it and every toast here landed on the default one —
      // the `'gold'` below asked for a colour it never got, silently.
      toast(`${res.chunks} sections indexed.`, { title: `Saved ${res.path}`, accent: 'good' })
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
      toast('Its text stays in the library, with a removal date.', {
        title: `${res.path} no longer answers`,
        accent: 'warn',
      })
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

  /**
   * Take every selected document out of retrieval, one call at a time.
   *
   * Sequential rather than concurrent, and that is the cheap option here as well as the
   * careful one: a removal is one SQL transaction with no embedding behind it, so twenty of
   * them cost nothing worth parallelising — while a failure in the middle of twenty parallel
   * requests leaves a set nobody can describe.
   *
   * Two failure shapes, handled differently on purpose. A document that is already gone is
   * one line of a report; the loop keeps going. A rejected password is the *gate*, and every
   * remaining call would be rejected too — so it stops on the first one and reports it once,
   * because twenty toasts saying the same thing is not twenty times the information.
   *
   * What survives a partial run is the selection: the removed ones drop out, the failures
   * stay ticked, so retrying is one press rather than finding them again.
   */
  async function dropSelected() {
    const paths = picked.value.map(d => d.path)
    if (!paths.length)
      return
    busy.value = true
    error.value = ''
    armedAll.value = false // it is acting now: the armed label must not survive the press
    progress.value = { done: 0, total: paths.length }
    let removed = 0
    const failed = []
    try {
      for (const path of paths) {
        try {
          await remove(path)
          selected.value.delete(path)
          removed++
          if (editing.value === path)
            cancel()
        }
        catch (e) {
          if (e.name === 'WrongPass') {
            onLocked(e)
            return
          }
          failed.push(path)
        }
        progress.value = { done: removed + failed.length, total: paths.length }
      }
      // Same sentence as a single removal, for the same reason: "deleted" would be a
      // promise the soft delete does not make.
      toast(
        failed.length
          ? `${failed.length} could not be removed and stay selected. The rest keep their text, with a removal date.`
          : 'Their text stays in the library, with a removal date.',
        {
          title: `${removed} ${removed === 1 ? 'document' : 'documents'} no longer answer`,
          accent: failed.length ? 'crit' : 'warn',
        },
      )
    }
    finally {
      busy.value = false
      progress.value = null
      onChanged()
    }
  }

  return { query, folder: pickedFolder, shown, kinds, folders, form, formEl, editing, open, busy, error, armed, arm, create, edit, cancel, save, drop, selected, picked, count, allShown, armedAll, armAll, toggle, toggleAll, clearSelection, dropSelected, progress }
}
