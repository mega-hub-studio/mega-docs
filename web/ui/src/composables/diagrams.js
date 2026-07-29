/* ══ use/diagrams.js — the renderer arrives late, and the viewer opens on tap ═══
   Three facts, one place:

     1. mermaid is 3.4 MB, so it is fetched the first time an answer actually draws
        something. `ready` flipping is what re-renders the answer and turns the fence
        into <nes-mermaid> — so the element never exists before the renderer does.
     2. A restored thread can already contain a diagram. The fetch used to happen only
        when an answer *arrived*, so a reload left it as source code, and a phone
        reloads far more often than it asks.
     3. In the answer a diagram is fitted (the shape, whole); the viewer shows it at
        the size it was drawn for. See diagram.js for why that size takes JS.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref } from 'vue'
import * as diagram from '../lib/diagram.js'

/**
 * @param {{ zoom: import("vue").Ref<HTMLDialogElement>, zoomBody: import("vue").Ref<Element> }} refs
 * @returns {{ ready: import("vue").Ref<boolean>, loadFor: (text: string) => void,
 *   drawn: (e: Event) => void, stepped: (e: Event) => void, open: (e: Event) => void,
 *   close: () => void }}
 */
export function useDiagrams({ zoom, zoomBody }) {
  const ready = ref(false)

  /** Fetch the renderer if this text contains a diagram and it isn't here yet. */
  function loadFor(text) {
    if (ready.value || !diagram.hasDiagram(text))
      return
    diagram.ready().then(ok => (ready.value = ok))
  }

  /**
   * A diagram just drew. The event is the library's and it bubbles, so one listener on
   *  the answer catches every diagram in it — including the ones inside v-html, which
   *  Vue never sees as components.
   */
  function drawn(e) {
    diagram.onRender(e.target)
  }

  /**
   * A walkthrough advanced. Same reason this is here rather than on each walkthrough: the
   * element arrives inside v-html, so one listener on the answer catches every one of them.
   */
  function stepped(e) {
    diagram.onStep(e)
  }

  function open(e) {
    if (e.type === 'keydown' && e.key !== 'Enter' && e.key !== ' ')
      return
    const host = e.target.closest?.('nes-mermaid')
    if (!host)
      return
    e.preventDefault() // Space would otherwise scroll the conversation
    if (diagram.zoomInto(zoomBody.value, host))
      zoom.value?.showModal()
  }

  function close() {
    zoom.value?.close()
    zoomBody.value?.replaceChildren() // a 1000-node SVG is not worth keeping around
  }

  return { ready, loadFor, drawn, stepped, open, close }
}
