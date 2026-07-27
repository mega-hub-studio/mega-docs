/* ══ use/statusline.js — the bottom strip, assembled from what is known ════════
   Pure presentation: in go four reactive inputs, out comes one object the markup
   renders field by field. Nothing here is estimated — a field whose input is missing
   is left empty and the markup drops it.

   It is its own file because it is the one piece of the app that turns numbers into
   claims about money, and a wrong claim there is worse than no claim.
   ═══════════════════════════════════════════════════════════════════════════ */

const LABEL = { error: "ERROR", running: "ASKING", done: "READY", queued: "IDLE" };

/**
 * @param {{ turns: import("vue").Ref<object[]>, busy: import("vue").Ref<boolean>,
 *   online: import("vue").Ref<boolean>, runtime: import("vue").Ref<object> }} src
 * @returns {import("vue").ComputedRef<object>}
 */
export function useStatusLine({ turns, busy, online, runtime }) {
  const { computed } = Vue;

  return computed(() => {
    const t = turns.value.at(-1);
    const state = !online.value ? "error" : busy.value ? "running" : t?.error ? "error" : t ? "done" : "queued";
    const line = {
      state,
      label: online.value ? LABEL[state] : "OFFLINE",
      tokens: "",
      refs: 0,
      elapsed: "",
      cost: "",
      costTitle: "",
    };
    // Mid-stream there is nothing true to say yet: the counts arrive with `done`.
    if (!t || t.streaming) return line;

    line.refs = t.citations.length;
    if (t.ms) line.elapsed = t.ms >= 1000 ? (t.ms / 1000).toFixed(1) + "s" : t.ms + "ms";

    const total = (t.in || 0) + (t.out || 0);
    if (total) {
      line.tokens = total.toLocaleString() + " tok";
      // Only claim a share of the window when the operator said how big it is.
      if (runtime.value.window > 0) {
        line.tokens += " · " + Math.round((total / runtime.value.window) * 100) + "%";
      }
    }

    if (t.cached) {
      line.cost = "cached · free";
      line.costTitle = "Served from the answer cache — no completion was bought";
    } else if (total && (runtime.value.priceIn || runtime.value.priceOut)) {
      const usd = ((t.in || 0) * runtime.value.priceIn + (t.out || 0) * runtime.value.priceOut) / 1e6;
      // Four decimals: one internal question costs a fraction of a cent, and rounding
      // it to $0.00 hides the only number anyone would act on.
      line.cost = "$" + usd.toFixed(4);
      line.costTitle =
        t.in + " in + " + t.out + " out at $" + runtime.value.priceIn + " / $" + runtime.value.priceOut + " per 1M";
    }
    return line;
  });
}
