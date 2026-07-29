/* ══ use/release.js — the notes behind the version badge ══════════════════════════
   Opening the dialog and grouping what comes back. Two decisions worth reading:

   The fetch happens once, on the first open, and the result is kept. Reopening the dialog
   costs nothing, and there is nothing to invalidate — the notes describe the binary that
   is answering, so they cannot change until it restarts, and a restart reloads the page's
   health anyway.

   Grouping lives here rather than in the component, because it is a branch and a component
   holding branches is a composable nobody wrote. What the component gets is an ordered
   array of `{ kind, notes }`, so its template is a v-for over facts with no decisions left
   in it — including the labels, which stay in the component because they are translated and
   a composable may not reach for another one's state.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref } from 'vue'
import { messages } from '../lib/i18n.js'
import { release } from '../lib/release.js'

// The kinds the app knows, in the order a reader wants them: what was added, what was
// repaired, then the rest. This is the catalogue's own key order rather than a second list —
// the two disagreed once and the dialog rendered `release.kind.ci` as a heading, because the
// generator's idea of a Conventional-Commit type is any `word:` prefix a commit carried.
const ORDER = Object.keys(messages.en.release.kind)

/**
 * @param {{ dialog: import("vue").Ref<HTMLDialogElement | null> }} deps the <dialog> the
 *   shell owns — the platform provides focus trapping and Escape, so neither is written here
 * @returns {{ groups: import("vue").Ref<Array<{kind: string, notes: object[]}>>,
 *   meta: import("vue").Ref<object>, loading: import("vue").Ref<boolean>,
 *   error: import("vue").Ref<string>, open: () => Promise<void>, close: () => void }}
 */
export function useRelease({ dialog }) {
  const groups = ref([])
  const meta = ref({ version: '', date: '', previous: '' })
  const loading = ref(false)
  const error = ref('')
  let loaded = false

  async function open() {
    dialog.value?.showModal()
    if (loaded || loading.value)
      return
    loading.value = true
    error.value = ''
    try {
      const r = await release()
      meta.value = { version: r.version, date: r.date, previous: r.previous }
      groups.value = group(r.notes)
      loaded = true
    }
    catch (e) {
      // Shown inside the dialog. A modal that opens empty reads as a broken app, so the
      // failure is named where the reader is already looking.
      error.value = e.message
    }
    finally {
      loading.value = false
    }
  }

  function close() {
    dialog.value?.close()
  }

  return { groups, meta, loading, error, open, close }
}

/**
 * Bucket the notes by kind, in reading order.
 *
 * @param {object[]} notes as generated — flat, newest first
 * @returns {Array<{kind: string, notes: object[]}>} non-empty buckets only
 */
function group(notes) {
  const byKind = new Map()
  for (const n of notes) {
    // A prefix with no label joins `other` rather than becoming a group nothing can name:
    // every kind reaching the component is one the catalogue translates, so the heading is a
    // word in the reader's language and never a key path. The note itself is untouched — its
    // scope and subject still render, which is what the generator refuses to drop.
    const kind = ORDER.includes(n.kind) ? n.kind : 'other'
    if (!byKind.has(kind))
      byKind.set(kind, [])
    byKind.get(kind).push(n)
  }
  return [...byKind.entries()]
    .map(([kind, list]) => ({ kind, notes: list }))
    .sort((a, b) => ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind))
}
