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
import { release } from '../lib/release.js'

// The order a reader wants, not the order git produced: what was added, what was repaired,
// then the rest. A kind absent from this list still renders — it sorts last, in the order it
// arrived — because a release that hides commits under an unrecognised prefix is exactly the
// silent omission the generator refuses to make.
const ORDER = ['feat', 'fix', 'perf', 'refactor', 'docs', 'style', 'test', 'build', 'chore', 'other']

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
    const kind = n.kind || 'other'
    if (!byKind.has(kind))
      byKind.set(kind, [])
    byKind.get(kind).push(n)
  }
  // Known kinds in ORDER, then anything unrecognised in arrival order — `indexOf` returning
  // -1 for both sides of a comparison leaves them where they were.
  const rank = k => (ORDER.includes(k) ? ORDER.indexOf(k) : ORDER.length)
  return [...byKind.entries()]
    .map(([kind, list]) => ({ kind, notes: list }))
    .sort((a, b) => rank(a.kind) - rank(b.kind))
}
