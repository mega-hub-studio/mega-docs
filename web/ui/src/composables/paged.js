/* ══ use/paged.js — one page of a list, and the numbers to reach the others ═════
   Three lists in this app grow without a ceiling and all three are read on a phone: the
   BA's queue, the library, and the remove-a-document drawer. Each one used to render every
   row it had, so a corpus of 200 documents was 200 rows between the Find field and anything
   below it.

   It takes a *getter*, not an array, for the reason every composable here does: `documents`
   and `queue.tickets` are replaced wholesale on each refresh, and a held array would page
   through the corpus as it was one save ago.

   The page is clamped on read rather than corrected on write, which is the whole reason
   `wanted` and `page` are two values. A filter that shrinks the list under the reader's feet
   — typing in Find is exactly that — must not leave them on an empty page 7, and clamping in
   a watcher would then *forget* page 7 when the filter is cleared. Reading it clamped gives
   both: the list is never blank, and backing out of a search puts the reader back where they
   were.

   What it does not do is fetch. The server caps its own payloads (100 tickets, 200
   documents) and this pages what arrived — `queueTotal` in lib/qa.js is what says so on
   screen, because a pager that silently ends at row 100 is the same lie as no pager at all.
   ═══════════════════════════════════════════════════════════════════════════ */
import { computed, ref } from 'vue'

/* How many numbered buttons the strip offers before it starts sliding. Five `.pg` boxes plus
   the two arrows measure 226px, which is the widest this may be and still fit a 390px screen
   with the card's padding on both sides. */
const WINDOW = 5

/**
 * @param {() => object[]} items getter over the whole list — see the header on why
 * @param {number} [size] rows per page
 * @returns {{ page: import("vue").ComputedRef<number>, pages: import("vue").ComputedRef<number>,
 *   slice: import("vue").ComputedRef<object[]>, numbers: import("vue").ComputedRef<number[]>,
 *   go: (n: number) => void }}
 */
export function usePaged(items, size = 10) {
  const wanted = ref(1)
  const pages = computed(() => Math.max(1, Math.ceil(items().length / size)))
  const page = computed(() => Math.min(wanted.value, pages.value))
  const slice = computed(() => items().slice((page.value - 1) * size, page.value * size))

  // The window slides so the current page stays near the middle, and stops at both ends
  // rather than running past them — `min` against `pages - span + 1` is what pins the last
  // window flush right, and the outer `max(1, …)` is what stops a list shorter than the
  // window from starting at zero or below.
  const numbers = computed(() => {
    const span = Math.min(WINDOW, pages.value)
    const first = Math.max(1, Math.min(page.value - Math.floor(span / 2), pages.value - span + 1))
    return Array.from({ length: span }, (_, i) => first + i)
  })

  const go = n => (wanted.value = Math.min(Math.max(1, n), pages.value))

  return { page, pages, slice, numbers, go }
}
