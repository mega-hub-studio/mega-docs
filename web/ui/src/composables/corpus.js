/* ══ use/corpus.js — what the engine knows, and what to ask it first ══════════
   Wraps library.js in reactive state. The starters live here rather than in the shell
   because they are derived from the corpus and nothing else: three hardcoded questions
   about the engine were a first screen that advertised the wrong subject.
   ═══════════════════════════════════════════════════════════════════════════ */
import { computed, ref } from "vue";
import { loadCorpus, starters as startersFor } from "../lib/library.js";

const LOADING = { state: "loading", docs: 0, chunks: 0, approved: 0, documents: [] };

/**
 * @returns {{ corpus: import("vue").Ref<object>, starters: import("vue").ComputedRef<string[]>,
 *   refresh: () => Promise<void> }}
 */
export function useCorpus() {
  const corpus = ref(LOADING);

  /** Never throws: loadCorpus resolves to a usable object in every case, including
   *  "can't reach the server", which the empty screen says out loud. */
  async function refresh() {
    corpus.value = await loadCorpus();
  }

  return { corpus, starters: computed(() => startersFor(corpus.value)), refresh };
}
