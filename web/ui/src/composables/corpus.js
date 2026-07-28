/* ══ use/corpus.js — what the engine knows ════════════════════════════════════
   Wraps library.js in reactive state, and nothing more: `corpus.documents` is the list
   the first screen ranks and the tree renders, so what to ask first is the finder's
   concern (composables/finder.js), not this one.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref } from "vue";
import { loadCorpus } from "../lib/library.js";

const LOADING = { state: "loading", docs: 0, chunks: 0, approved: 0, documents: [] };

/**
 * @returns {{ corpus: import("vue").Ref<object>, refresh: () => Promise<void> }}
 */
export function useCorpus() {
  const corpus = ref(LOADING);

  /** Never throws: loadCorpus resolves to a usable object in every case, including
   *  "can't reach the server", which the empty screen says out loud. */
  async function refresh() {
    corpus.value = await loadCorpus();
  }

  return { corpus, refresh };
}
