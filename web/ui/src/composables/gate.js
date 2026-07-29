/* ══ use/gate.js — the write gate: one password, two actions behind it ══════════
   Confirming an answer and importing a document are the two things that change what the
   engine will say, and both are behind BA_PASS. The gate is its own composable because
   *both* of them fail the same way, and that failure has to be handled in one place:

     wrong password  →  the form says so, and the state goes back to locked
     writes disabled →  403, which is not a wrong password and must not read as one

   The password is checked before it is stored. Storing it unchecked is how a typo used to
   survive until the first upload and then look like a broken import.
   ═══════════════════════════════════════════════════════════════════════════ */
import { ref } from 'vue'
import * as qa from '../lib/qa.js'
import * as upload from '../lib/upload.js'

/**
 * @param {{ toast: Function }} deps
 * @returns {{ unlocked, passInput, unlocking, unlockError, unlock: () => Promise<void>,
 *   fail: (e: Error, prefix?: string) => void }}
 */
export function useGate({ toast }) {
  const unlocked = ref(!!qa.pass())
  const passInput = ref('')
  const unlocking = ref(false)
  const unlockError = ref('')

  async function unlock() {
    const candidate = passInput.value.trim()
    if (!candidate)
      return
    unlockError.value = ''
    unlocking.value = true
    try {
      if (!(await upload.verify(candidate))) {
        unlockError.value = 'That password does not open the gate. Reads still work.'
        return
      }
      qa.setPass(candidate)
      unlocked.value = !!qa.pass()
      passInput.value = ''
    }
    catch (e) {
      unlockError.value = e.message
    }
    finally {
      unlocking.value = false
    }
  }

  /**
   * What to do when a write comes back refused. A refused password has to say so *in the
   *  form*, not only in a toast: the thing being read is the card that just vanished.
   */
  function fail(e, prefix = '') {
    if (e instanceof qa.WrongPass) {
      unlocked.value = false
      if (prefix)
        unlockError.value = prefix + e.message
      toast(e.message, { title: 'Locked', accent: 'crit' })
    }
    else {
      toast(e.message, { accent: 'crit' })
    }
  }

  return { unlocked, passInput, unlocking, unlockError, unlock, fail }
}
