/* ══ use/settings.js — everything a reader may change, in one place ═════════════
   The gear opens one panel and that panel is the whole surface: which model answers, which
   language the app is in, whether it makes noise. Two of those existed already and were two
   different controls in two different corners — the language a button in the bar, the sound a
   flag written once in main.js with no way to change it afterwards.

   What is *not* here: anything only an operator can answer. The knobs an instance was started
   with are read-only, password-gated and already have a screen (`/#/admin`); the drawer links
   to it rather than restating it, because a value with two homes is a value that disagrees
   with itself. So the split is by *who decides*, not by subject:

     drawer   the reader's choices, kept in this browser
     /#/admin the instance's configuration, kept in .env and read at startup

   The model choice is the one that leaves the browser: it rides on every question, and the
   server refuses anything it does not offer — so a stale pick from last week is answered with
   a 400 rather than silently becoming something else. `reconcile` is what stops that being a
   thing a reader has to think about.
   ═══════════════════════════════════════════════════════════════════════════ */
import { setMute } from '8bit-nes'
import { computed, ref } from 'vue'

const MODEL_KEY = 'md_model'
const MUTE_KEY = 'nes_mute' // the library's own key: it reads this on load

/**
 * @param {{ models: () => object[] }} deps `models` is a getter, because /api/health answers
 *   after the first render and a held array would be the empty one it started with.
 * @returns {object} the drawer's element and state, plus the model every question carries
 */
export function useSettings({ models }) {
  // The <dialog>. Native, so Escape, the focus trap and the backdrop are the platform's job
  // rather than three things to reimplement — the same reason the diagram viewer is one.
  const el = ref(null)
  const model = ref(localStorage.getItem(MODEL_KEY) || '')
  const muted = ref(localStorage.getItem(MUTE_KEY) === 'true')

  function open() {
    el.value?.showModal()
  }

  function close() {
    el.value?.close()
  }

  /**
   * Which model to send, as a name the server will accept: the stored pick if this instance
   * still offers it, otherwise the instance default, otherwise nothing at all.
   *
   * The empty string is meaningful and is not a bug: it means "whatever you are configured
   * with", which is the honest request from a client that was never told there was a choice.
   */
  const picked = computed(() => {
    const list = models()
    if (!list.length)
      return ''
    return list.some(m => m.name === model.value) ? model.value : list[0].name
  })

  /** The picked model's own numbers, for the strip: window and price, or zeros for unknown. */
  const current = computed(() => models().find(m => m.name === picked.value) ?? {})

  function pick(name) {
    model.value = name
    localStorage.setItem(MODEL_KEY, name)
  }

  /**
   * Sound is the library's, including where the preference lives — `setMute` writes the same
   * key it reads at load, so this only mirrors it for the switch to bind to.
   * @param {boolean} on
   */
  function mute(on) {
    muted.value = on
    setMute(on)
  }

  return { el, open, close, model, picked, current, pick, muted, mute }
}
