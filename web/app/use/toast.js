/* ══ use/toast.js — the one design-system helper a component may need ══════════
   `toast()` cannot be imported: the pinned, integrity-checked CDN URL for the design
   system lives in index.html alone, so boot() receives the helper and provides it.

   This wraps the inject so a component asks for it by name once, and so a component
   mounted without a provider (a test harness, a future second app) degrades to a no-op
   instead of throwing on the first success message.
   ═══════════════════════════════════════════════════════════════════════════ */

/** @returns {(html: string, opts?: object) => void} */
export function useToast() {
  const { inject } = Vue;
  return inject("toast", () => {});
}
