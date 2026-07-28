/* ══ lang.js — the only door to i18n a component may use ══════════════════════
   `const { t } = useT()`. That is the whole surface.

   Why a wrapper instead of `useI18n()` directly, which is what vue-i18n documents:

   1. **Scope.** `useI18n()` with no argument creates a *local* scope for that component,
      and a component whose parent has no i18n instance yet throws
      "[intlify] Not found parent scope". Pinning `useScope: 'global'` here means no
      component can get that wrong, and there is exactly one catalogue for the app rather
      than one per component that asked differently.
   2. **One thing to change.** Every string in the app goes through this function, so
      swapping vue-i18n for anything else — or adding a third language, or a per-key
      fallback — is an edit to this file and nowhere else.

   The layer rules still hold: this is a composable (reactive state and every branch),
   lib/i18n.js is the data, and main.js does the wiring. A component never imports
   vue-i18n, and never sees a locale string unless it renders one.
   ═══════════════════════════════════════════════════════════════════════════ */
import { useI18n } from 'vue-i18n'
import { LANGS, storeLang } from '../lib/i18n.js'

/**
 * @returns {{ t: Function, lang: import("vue").WritableComputedRef<string>,
 *   langs: string[], setLang: (lang: string) => void }}
 *   `t` translates. `lang` is the current locale, readable in a template. `setLang`
 *   switches and remembers, and is what the header's toggle calls.
 */
export function useT() {
  const { t, locale } = useI18n({ useScope: 'global' })

  function setLang(lang) {
    if (!LANGS.includes(lang))
      return
    locale.value = lang
    storeLang(lang)
    // The document's own language, so a screen reader announces the right one and CSS can
    // hook `:lang()`. The guide's pages set this too, for the same reason.
    document.documentElement.lang = lang
  }

  return { t, lang: locale, langs: LANGS, setLang }
}
