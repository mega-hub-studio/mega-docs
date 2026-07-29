/* ══ use/statusline.js — the bottom strip, assembled from what is known ════════
   Pure presentation: in go four reactive inputs, out comes one object the markup
   renders field by field. Nothing here is estimated — a field whose input is missing
   is left empty and the markup drops it.

   It is its own file because it is the one piece of the app that turns numbers into
   claims about money, and a wrong claim there is worse than no claim.
   ═══════════════════════════════════════════════════════════════════════════ */

import { computed } from 'vue'

const LABEL = { error: 'ERROR', running: 'ASKING', done: 'READY', queued: 'IDLE' }

/**
 * @param {{ turns: import("vue").Ref<object[]>, busy: import("vue").Ref<boolean>,
 *   online: import("vue").Ref<boolean>, runtime: import("vue").Ref<object>,
 *   model: import("vue").ComputedRef<object> }} src
 *   `model` is the picked model's own window and price. The instance-wide numbers in `runtime`
 *   describe the default one, so on an instance with a picker they are the wrong pair for
 *   every other choice — a percentage of the wrong window is worse than no percentage.
 * @returns {import("vue").ComputedRef<object>}
 */
export function useStatusLine({ turns, busy, online, runtime, model }) {
  return computed(() => {
    const t = turns.value.at(-1)
    const state = !online.value ? 'error' : busy.value ? 'running' : t?.error ? 'error' : t ? 'done' : 'queued'
    const line = {
      state,
      label: online.value ? LABEL[state] : 'OFFLINE',
      tokens: '',
      refs: 0,
      elapsed: '',
      cost: '',
      costTitle: '',
      // "3/8" — the thread as the model read it. Empty for a first question, because 0/0 is a
      // memory figure about nothing.
      recall: '',
    }
    // Mid-stream there is nothing true to say yet: the counts arrive with `done`.
    if (!t || t.streaming)
      return line

    line.refs = t.citations.length
    if (t.ms)
      line.elapsed = t.ms >= 1000 ? `${(t.ms / 1000).toFixed(1)}s` : `${t.ms}ms`

    // The window and the prices of the model that answered *this* turn, falling back to the
    // instance-wide pair for an answer produced before there was a picker. Reading the picked
    // model instead would price a two-day-old turn at whatever is selected right now.
    const m = t.model ? (runtime.value.models.find(x => x.name === t.model) ?? {}) : {}
    const window = m.window ?? model.value.window ?? runtime.value.window
    const priceIn = m.price_in ?? model.value.price_in ?? runtime.value.priceIn
    const priceOut = m.price_out ?? model.value.price_out ?? runtime.value.priceOut

    const total = (t.in || 0) + (t.out || 0)
    if (total) {
      line.tokens = `${total.toLocaleString()} tok`
      // Only claim a share of the window when the operator said how big it is.
      if (window > 0) {
        line.tokens += ` · ${Math.round((total / window) * 100)}%`
      }
    }

    // How much of the thread the answer was allowed to read. Shown only once there is a thread
    // to trim: on the first question of a conversation there is nothing to report, and a
    // "0/0" beside a fresh answer reads as a failure.
    if (t.recall?.offered)
      line.recall = `${t.recall.kept}/${t.recall.offered}`

    if (t.cached) {
      line.cost = 'cached · free'
      line.costTitle = 'Served from the answer cache — no completion was bought'
    }
    else if (total && (priceIn || priceOut)) {
      const usd = ((t.in || 0) * priceIn + (t.out || 0) * priceOut) / 1e6
      // Four decimals: one internal question costs a fraction of a cent, and rounding
      // it to $0.00 hides the only number anyone would act on.
      line.cost = `$${usd.toFixed(4)}`
      line.costTitle = `${t.in} in + ${t.out} out at $${priceIn} / $${priceOut} per 1M`
    }
    return line
  })
}
