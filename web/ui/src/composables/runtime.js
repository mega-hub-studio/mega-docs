/* ══ use/runtime.js — is it up, may a BA write, and what does a token cost ══════
   Everything /api/health reports, kept in one place because it is all answered by one
   request. Two of the three are load-bearing:

     online  drives the light in the header *and* the status line's OFFLINE state
     writes  tells BA mode it is read-only before someone types an answer into it
     runtime the model name and the prices the status line may show — nothing else. There
             was a guide address here too; the binary no longer links out to the guide, so
             /api/health no longer reports one and neither does this.

   Zero means unknown, and stays zero. The strip prints nothing rather than a zero,
   because an unmeasured cost and a cost of nothing are different facts.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref } from 'vue'
import { health } from '../lib/chat.js'

/**
 * @returns {{ online: import("vue").Ref<boolean>, writes: import("vue").Ref<boolean>,
 *   runtime: import("vue").Ref<object>, check: () => Promise<void>, watchNetwork: () => void }}
 */
export function useRuntime() {
  const online = ref(true) // optimistic: the first check has not answered yet
  const writes = ref(false) // pessimistic: never offer a write surface we cannot prove
  const admin = ref(false) // same, and for the same reason: no surface until it is proven
  const runtime = ref({ model: '', window: 0, priceIn: 0, priceOut: 0 })

  async function check() {
    const h = await health()
    online.value = h.online
    writes.value = h.writes
    admin.value = h.admin
    runtime.value = {
      model: h.model,
      window: h.window,
      priceIn: h.priceIn,
      priceOut: h.priceOut,
    }
  }

  /**
   * The browser knows about the network before a request fails, so listen — but only
   *  trust it in one direction: "offline" is a fact, "online" only means the interface
   *  came back, so it triggers a real check.
   */
  function watchNetwork() {
    addEventListener('online', check)
    addEventListener('offline', () => (online.value = false))
  }

  return { online, writes, admin, runtime, check, watchNetwork }
}
