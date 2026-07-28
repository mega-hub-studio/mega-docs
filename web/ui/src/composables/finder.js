/* ══ finder.js — type to narrow the documents on the first screen ═════════════
   The empty screen used to show three pre-built sentences and stop there: with seven
   documents indexed, four of them had no row and no way to reach one. The palette is the
   whole corpus instead, ranked, with a field that narrows it.

   State is one string. The ranking and the matching are pure functions in library.js, so
   this holds no branch — which is also what keeps the component free of one.
   ═══════════════════════════════════════════════════════════════════════════ */
import { computed, ref } from "vue";
import { rankDocs } from "../lib/library.js";

/* How many rows the menu draws at once.
   The library's `.palette-list` caps itself at 340px with `overflow-y: auto`, which is
   right for a palette in a modal and wrong in the page: measured on an iPhone 14 it showed
   three of seven documents behind a *nested* scrollbar — the same four unreachable
   documents this screen exists to fix, now hidden by a scroll trap instead of a slice.
   So the app un-caps the height (styles.css) and caps the row count here instead, at
   roughly the same visual budget — and the header says `shown / total`, so a corpus larger
   than this never gets truncated silently. */
const MAX_ROWS = 8;

/**
 * @param {{ documents: () => object[] }} deps `documents` is a getter, not an array:
 *   the corpus is refreshed after every ingest and import, and a held array would keep
 *   listing what was indexed a minute ago.
 * @returns {{ query: import("vue").Ref<string>, matches: import("vue").ComputedRef<object[]>,
 *   shown: import("vue").ComputedRef<object[]> }}
 */
export function useFinder({ documents }) {
  const query = ref("");
  const matches = computed(() => rankDocs(documents(), query.value));
  return { query, matches, shown: computed(() => matches.value.slice(0, MAX_ROWS)) };
}
