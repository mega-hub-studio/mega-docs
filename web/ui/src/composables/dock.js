/* ══ use/dock.js — whether the question box is in the way ══════════════════════
   One boolean, and it exists because of one measurement: on a 390×844 phone the dock is the
   scope line, the prompt and the status line stacked, and `main` reserves every pixel of it
   as bottom padding (`--dock-h`). That is roughly a fifth of the screen held open for a
   control the reader is not using while they read a long answer — and an answer with a table
   or a diagram in it is exactly when they need the room and exactly when they are not typing.

   So it collapses to its handle, and nothing else has to know: `viewport.js` measures the
   real dock with a ResizeObserver, so `--dock-h` shrinks on its own and `main` gives the
   space back. No second source of truth for the height, and no number in this file.

   Two decisions worth keeping:

   · **Hidden, not unmounted.** The template uses `v-show`, so `<nes-chat-prompt>` stays the
     same element — `useConversation` sets `busy` on it as an attribute through a watcher, and
     an answer must be able to keep streaming while the box that asked for it is out of sight.
   · **Not remembered.** A collapsed dock restored on load is a reader opening the app and
     finding no way to ask it anything. This is a gesture for the answer in front of you, so
     it lasts as long as that answer is.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref } from 'vue'

/**
 * @returns {{ collapsed: import("vue").Ref<boolean>, toggle: () => void, show: () => void }}
 *   `show` is for the callers that must not leave the reader looking at a hidden prompt —
 *   starting a new question is the one that matters.
 */
export function useDock() {
  const collapsed = ref(false)

  const toggle = () => {
    collapsed.value = !collapsed.value
  }
  const show = () => {
    collapsed.value = false
  }

  return { collapsed, toggle, show }
}
