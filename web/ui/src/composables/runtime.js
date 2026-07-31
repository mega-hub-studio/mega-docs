/* ══ use/runtime.js — is it up, may a BA write, and what does a token cost ══════
   Everything /api/health reports, kept in one place because it is all answered by one
   request. Two of the three are load-bearing:

     online  drives the light in the header *and* the status line's OFFLINE state
     writes  tells BA mode it is read-only before someone types an answer into it
     runtime the model name, the prices and the deployed revision the status line may
             show — nothing else. There
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
  // Whether this instance can look outside the documents. False until proven, like the two
  // above: a switch offering something the server cannot do is worse than no switch.
  const search = ref(false)
  const runtime = ref({
    model: '',
    window: 0,
    priceIn: 0,
    priceOut: 0,
    models: [],
    // What the engine is tuned to, published so the settings panel can show it without a
    // password: the floor on sections per answer, the share of the window a thread may take,
    // the share the retrieved sections may take, and cached rows.
    engine: { topK: 0, threadShare: 0, contextShare: 0, cacheKeep: 0 },
    version: '',
    release: '',
  })

  async function check() {
    const h = await health()
    online.value = h.online
    writes.value = h.writes
    admin.value = h.admin
    search.value = h.search
    runtime.value = {
      model: h.model,
      window: h.window,
      priceIn: h.priceIn,
      priceOut: h.priceOut,
      // What a reader may pick between. One entry (or none) is an instance with nothing to
      // choose, and the settings drawer says so rather than offering a menu of one.
      models: h.models ?? [],
      engine: h.engine,
      // Which commit is serving this page. It belongs with the runtime facts rather than in
      // a composable of its own: it arrives in the same request and answers the same kind of
      // question — what is this instance, right now.
      version: h.version,
      // The tag that commit was cut from, which is what the badge prints. It rides along here
      // for the same reason the commit does: it arrives in this request and answers the same
      // kind of question — what is this instance, right now. The notes behind it are a
      // separate route and a separate composable, because they are fetched on a click.
      release: h.release,
    }
  }

  /**
   * The browser knows about the network before a request fails, so listen — but only
   *  trust it in one direction: "offline" is a fact, "online" only means the interface
   *  came back, so it triggers a real check.
   *
   * A refocus is the third signal, and the only one that catches a deploy: /api/health's body
   * is built once at process start, so a tab left open all day reports whatever was true when
   * it loaded — a green light over a dead server, or last week's model list. An interval would
   * poll a constant to learn the same thing; coming back to the tab is when it matters.
   */
  function watchNetwork() {
    addEventListener('online', check)
    addEventListener('offline', () => (online.value = false))
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible')
        check()
    })
  }

  return { online, writes, admin, search, runtime, check, watchNetwork }
}
