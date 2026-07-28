/* ══ use/importer.js — dropping documents into the corpus ══════════════════════
   The whole import, minus the markup: which files were accepted, where they land, how far
   along it is, and what came back.

   Two things here are load-bearing rather than decorative:

     progress   real counts, because one request per file is what makes the bar *true*.
                A single POST for the batch could only be animated by guessing, and a bar
                that invents its position claims "nearly done" while the last file has
                not started.
     imported   partial success, reported. Eight files where one is a PDF is seven
                indexed and the eighth named — in one list, because to the person who
                dropped them there was one list.
   ═══════════════════════════════════════════════════════════════════════════ */
import { computed, ref } from 'vue'
import * as upload from '../lib/upload.js'

/**
 * @param {{ documents: () => object[], toast: Function, onLocked: (e: Error) => void,
 *   onIndexed: () => void }} deps
 *   documents is a getter, not an array: the folder suggestions follow the corpus, which
 *   the shell replaces wholesale after every import.
 */
export function useImporter({ documents, toast, onLocked, onIndexed }) {
  const importDir = ref('') // the folder this batch lands in — the scope a reader browses
  const importing = ref(false)
  const progress = ref({ done: 0, total: 0 })
  const dragging = ref(false) // a drop target that doesn't light up reads as inert
  const imported = ref(null) // {uploaded, failed, chunks} from the last import

  /**
   * The folders that already exist, so the picker suggests the structure rather than
   *  inviting a fourth spelling of "engineering".
   */
  const folders = computed(() => upload.folders(documents()))

  async function importDocs(files) {
    const { ok, rejected } = upload.sort(files)
    dragging.value = false
    if (!ok.length) {
      toast(`<b>Nothing to import.</b> Only ${upload.ACCEPT} — convert a PDF first.`, { accent: 'warn' })
      return
    }
    importing.value = true
    imported.value = null
    progress.value = { done: 0, total: ok.length }
    try {
      const r = await upload.send(ok, importDir.value, (done, total) => {
        progress.value = { done, total }
      })
      r.failed = [...r.failed, ...rejected.map(f => ({ name: f.name, error: `not ${upload.ACCEPT}` }))]
      imported.value = r
      if (r.uploaded.length) {
        toast(`<b>${r.uploaded.length} document(s) indexed.</b> ${r.chunks} sections — ask about them now.`, {
          accent: 'good',
        })
        onIndexed()
      }
      else {
        toast('<b>Nothing was indexed.</b> See the list below.', { accent: 'crit' })
      }
    }
    catch (e) {
      onLocked(e)
    }
    finally {
      importing.value = false
    }
  }

  /** The file picker and a drop end in the same place. */
  function pickDocs(e) {
    importDocs(e.target.files)
    e.target.value = '' // so picking the same file twice still fires
  }

  /* ── removing a document ──
     Two steps, never one. Import is additive and a mistake costs an ingest; removal takes
     an answer's source away from every future reader, and the button that does it sits in a
     list of paths that differ by one word. So the path is shown back verbatim and confirmed
     — which is exactly what the library's `.perm` recipe is for — and only one document can
     be pending at a time, because a queue of pending deletions is a way to confirm the
     wrong one. `removing` is the path in flight, so its own row can say so without a
     second flag. */
  const pending = ref('') // the path awaiting confirmation, or ''
  const removing = ref('') // the path whose request is in flight, or ''

  function askRemove(path) {
    pending.value = path
  }

  function cancelRemove() {
    pending.value = ''
  }

  async function confirmRemove() {
    const path = pending.value
    if (!path)
      return
    pending.value = ''
    removing.value = path
    try {
      const r = await upload.remove(path)
      // Name where it went. "Deleted" would be a lie — the file is in the corpus's own
      // .trash/, which is the difference between a mistake and a loss.
      toast(r.trash
        ? `<b>${r.path} removed.</b> The file is in <code>${r.trash}</code> if you need it back.`
        : `<b>${r.path} removed.</b> Its file was already gone; the index is clean.`, { accent: 'good' })
      onIndexed()
    }
    catch (e) {
      onLocked(e)
    }
    finally {
      removing.value = ''
    }
  }

  return {
    accept: upload.ACCEPT,
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
  }
}
