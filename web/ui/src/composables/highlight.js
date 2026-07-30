/* ══ use/highlight.js — paint the code blocks once the answer has rendered ══════
   The whole composable is a timing fact, which is why it is not two lines in the shell.

   `turnHtml` only emits <nes-code> once a turn has stopped streaming, so at the moment an
   answer settles the elements this paints do not exist yet — Vue re-renders after. `nextTick`
   is that gap, and it is a Vue import, which rule 9 keeps out of `src/lib`. So the wait lives
   here and lib/highlight.js stays a plain module.

   It paints the document rather than one turn: a restored thread can already hold code, and
   `paint` skips anything already done, so "everything not yet painted" is both the cheapest
   query and the one that covers a reload. Nothing is fetched when there is no code.
   ═══════════════════════════════════════════════════════════════════════════ */
import { nextTick } from 'vue'
import { paint } from '../lib/highlight.js'

/** @returns {{ paintCode: () => Promise<void> }} */
export function useHighlight() {
  async function paintCode() {
    await nextTick()
    await paint(document)
  }
  return { paintCode }
}
