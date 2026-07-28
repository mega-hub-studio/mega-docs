/* ══ router.js — which screen, in the address ═══════════════════════════════════
   Two screens, and now one place that says which one is showing: the URL. It used to be
   a `mode` ref in App.vue mirrored into localStorage, which meant the screen you were
   looking at could not be linked, bookmarked or backed out of — and telling a BA where
   the queue is was a sentence about which button to press rather than an address.

   Three decisions worth reading before changing anything.

   1. Hash history, not `createWebHistory`. The binary serves exactly one HTML file, at
      `GET /{$}`, and `TestGuideRoutesAreNotServed` asserts every other path 404s — the
      app is one surface and must never read as a second copy of the guide. Clean URLs
      need the opposite: a catch-all answering `/ba` with the shell, which is that test
      inverted. A hash needs no server route at all, so `…/#/ba` survives a reload with
      nothing to configure and nothing on the Go side to keep in agreement.
   2. The queue is lazy. `/ba` pulls the gate, the tickets, the importer and the document
      tree behind it; a dynamic import gives Rollup its own chunk, so a DEV who only asks
      questions never downloads any of it. Same rule mermaid follows in lib/diagram.js.
   3. The landing screen is remembered; the current one is not. `/` resolves to whichever
      screen was last open, because a BA reopening the app wants the queue and not the
      prompt. After that the address is the only truth — no component keeps a copy.
   ═══════════════════════════════════════════════════════════════════════════ */
import { createRouter, createWebHashHistory } from 'vue-router'
import AskScreen from './components/AskScreen.vue'

// Written on every arrival, read only by `/`. A new key rather than the old `ke.mode`,
// because what is stored is a route name now and not dev/ba — the same key holding two
// vocabularies is how a stale browser lands nowhere.
const KEY = 'ke.screen'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    // A redirect record, not a `beforeEach` that pushes: a guard which sends `/`
    // somewhere is the shape that loops, and neither target below redirects.
    { path: '/', redirect: () => (localStorage.getItem(KEY) === 'ba' ? '/ba' : '/ask') },
    { path: '/ask', name: 'ask', component: AskScreen },
    { path: '/ba', name: 'ba', component: () => import('./components/BaScreen.vue') },
    // Lazy for the same reason /ba is: nobody asking questions needs the settings screen,
    // and a dynamic import gives Rollup its own chunk. Registered unconditionally — the
    // screen itself discovers whether this instance has an admin surface, because the route
    // table is static and /api/health is not.
    { path: '/admin', name: 'admin', component: () => import('./components/AdminScreen.vue') },
    // An old link, or a typo in the hash, lands on the prompt — rather than a shell with
    // an empty <main> and nothing in the console to explain it.
    { path: '/:unknown(.*)', redirect: '/ask' },
  ],
})

router.afterEach((to) => {
  localStorage.setItem(KEY, to.name)
})
