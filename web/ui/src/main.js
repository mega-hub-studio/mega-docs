/* ══ main.js — the entry: three lines of setup and a mount ══════════════════════
   Everything the browser needs is imported here, in load order: the design system's
   stylesheet (which pulls in its own fonts), its custom elements, this app's layout CSS,
   and the shell. Vite bundles and content-hashes all of it, so the page it produces names
   no version and fetches nothing from a CDN.
   ═══════════════════════════════════════════════════════════════════════════ */
// One import of the design system, for both halves of what it is: `setMute` is a helper,
// and the module's side effect is defining <nes-icon>, <nes-chat-prompt>, <nes-tree> and
// <nes-mermaid>. Importing it twice would say otherwise.
import { setMute } from '8bit-nes'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import App from './App.vue'
import { messages, preferredLang } from './lib/i18n.js'
import { router } from './router.js'

import '8bit-nes/all.css'
import './styles.css'

// The chiptune SFX default to on. In a work tool opened on a phone the first send
// shouldn't make noise — default to silent, but only on a first visit, so a deliberate
// un-mute is remembered.
if (localStorage.getItem('nes_mute') === null)
  setMute(true)

const app = createApp(App)

// The one place the i18n instance is built. Components reach it through `useT()`
// (composables/lang.js), which pins the global scope — never `useI18n()` directly.
//
// `legacy: false` is the Composition API mode, and it is what makes the tree-shaken build
// possible: the Options API half of vue-i18n is what carries most of its weight.
// `fallbackLocale` is English so a key missing from the Vietnamese catalogue renders a real
// sentence rather than `empty.title`.
const i18n = createI18n({
  legacy: false,
  locale: preferredLang(),
  fallbackLocale: 'en',
  messages,
  // A key that exists in neither catalogue is a bug, not a runtime condition — but it must
  // not fill the console on a phone during a demo. Warn once in dev, silent in the bundle.
  missingWarn: import.meta.env.DEV,
  fallbackWarn: false,
})
app.use(i18n)

// Which screen is showing, and the two components that read it: `app.use` is what
// registers <router-view> and <router-link> globally, which is also why eslint's
// `vue/no-undef-components` has to be told they exist. The routes and the three decisions
// behind them are in router.js.
app.use(router)

// Before first paint, so the document never announces the wrong language.
document.documentElement.lang = i18n.global.locale.value
// No isCustomElement here: templates are compiled at *build* time, so that decision
// lives in vite.config.js. Setting it at runtime would be too late — the compiled render
// functions would already be resolving <nes-icon> as a Vue component.
app.mount('#app')
