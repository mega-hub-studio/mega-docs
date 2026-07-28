/* ══ use/admin.js — the Admin screen, and every branch in it ════════════════════
   Read-only by design, so this holds less than it looks: which tab, whether the password
   opened the gate, and the one list the screen renders.

   Three things are decided here rather than in the markup:

     grouped   the flat inventory arrives in .env.example's order; the screen shows it in
               groups. Grouping is a fold over an array, which is a branch, which belongs
               in a composable and not in a template.
     refused   a wrong password re-locks and says so in the form. An *absent* surface does
               not: there is no password that would work, so inviting a retry is a lie.
     tab       one at a time, and the name rather than an index — an index reorders when a
               tab is added and nothing tells you.

   It has its own unlock rather than reaching for useGate: that gate is BA_PASS, this is
   ADMIN_PASS, and a composable never reaches for another composable's state.
   ═══════════════════════════════════════════════════════════════════════════ */
import { computed, ref } from 'vue'
import * as admin from '../lib/admin.js'

/**
 * @param {{ toast: Function }} deps
 */
export function useAdmin({ toast }) {
  const tab = ref('settings')
  const unlocked = ref(!!admin.pass())
  const passInput = ref('')
  const unlocking = ref(false)
  const unlockError = ref('')
  const absent = ref(false) // ADMIN_PASS unset here: no form, no retry
  const settings = ref([])

  /**
   * The inventory, folded into the groups .env.example uses — in first-seen order, so the
   * screen and the file read the same way round and neither carries a list of group names.
   */
  const grouped = computed(() => {
    const out = new Map()
    for (const s of settings.value) {
      if (!out.has(s.group))
        out.set(s.group, [])
      out.get(s.group).push(s)
    }
    return [...out].map(([name, rows]) => ({ name, rows }))
  })

  async function fetchSettings() {
    try {
      settings.value = await admin.load()
      unlocked.value = true
    }
    catch (e) {
      if (e instanceof admin.Absent) {
        absent.value = true
        unlocked.value = false
        return
      }
      if (e instanceof admin.Refused) {
        unlocked.value = false
        unlockError.value = e.message
        return
      }
      toast(`<b>${e.message}</b>`, { accent: 'crit' })
    }
  }

  async function unlock() {
    const candidate = passInput.value.trim()
    if (!candidate)
      return
    unlockError.value = ''
    unlocking.value = true
    try {
      if (!(await admin.verify(candidate))) {
        unlockError.value = 'That password does not open the admin screen.'
        return
      }
      admin.setPass(candidate)
      passInput.value = ''
      await fetchSettings()
    }
    catch (e) {
      unlockError.value = e.message
    }
    finally {
      unlocking.value = false
    }
  }

  return {
    tab,
    unlocked,
    passInput,
    unlocking,
    unlockError,
    absent,
    grouped,
    fetchSettings,
    unlock,
  }
}
