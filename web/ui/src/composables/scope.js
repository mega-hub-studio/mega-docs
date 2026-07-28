/* ══ use/scope.js — which part of the corpus answers ═══════════════════════════
   The smallest composable here, and the one with the sharpest edge: the scope is
   half the answer cache's key on the server, so "booking/" and "/booking" must not
   be two scopes. Canonicalising is the engine's job (rag.Scope); this side's job is
   to store exactly what the tree emitted and nothing of its own invention.
   ═══════════════════════════════════════════════════════════════════════════ */

import { ref } from 'vue'

const KEY = 'ke.scope' // a reader working through one folder asks several questions

/**
 * @returns {{ scope: import("vue").Ref<string>, setScope: (s: string) => void }}
 */
export function useScope() {
  const scope = ref(localStorage.getItem(KEY) || '')

  /** Narrow the next question to one folder or document; "" widens it back to all. */
  function setScope(next) {
    scope.value = next || ''
    // Stored, because a reload that silently widened the scope would answer a narrow
    // question from the whole corpus and look like the same feature working.
    if (scope.value)
      localStorage.setItem(KEY, scope.value)
    else localStorage.removeItem(KEY)
  }

  return { scope, setScope }
}
