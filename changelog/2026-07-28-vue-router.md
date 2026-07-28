# 2026-07-28 — vue-router: the screen is an address, and the three things that decided how

The app had two screens and no way to say which one you were on. `App.vue` held
`const mode = ref(localStorage.getItem('ke.mode') === 'ba' ? 'ba' : 'dev')`, so the queue
could not be linked, bookmarked or backed out of, and pointing a BA at it was a sentence
about which button to press. It is `vue-router` now — `/#/ask` and `/#/ba`, in
`web/ui/src/router.js` — and `ke.mode` is gone.

Three decisions, each one a real fork, recorded so nobody re-derives them.

## 1 · Hash history, not `createWebHistory`

Clean URLs need the server to answer `/ba` with the shell. That is
`TestGuideRoutesAreNotServed` inverted: it asserts `/docs`, `/dev`, `/deploy` and
`/llms.txt` **404**, because the app is one surface and must never read as a second copy of
the guide. A catch-all would have meant rewriting that test's premise and serving HTML for
a mistyped `/api/` path; naming `GET /ba` in the mux instead would have put the route table
in two languages, which is rule 17.

`createWebHashHistory()` needs no route on the Go side at all, so a bookmarked `…/#/ba`
survives a reload with nothing to configure and nothing to keep in agreement. The cost is
the `#` in the address, and this app is a tailnet SPA with no SEO and no deep-link surface
outside itself. If clean URLs are ever wanted: one fallback route in `internal/server`,
plus a rewrite of that test, plus a reason better than taste.

## 2 · The state stays in the shell; the router only picks the screen

The obvious shape — route components that compose their own state — was rejected on a
concrete failure: `useConversation` holds the in-flight `AbortController`, and the router
**unmounts the screen you leave**, so an answer would die the moment a DEV looked at the
queue. `provide/inject` and a props bus over `<router-view>` were both rejected too: this
tree's rule is that a component is a contract (props in, events out), and a bus hands each
screen the other's props — which Vue then writes onto its root element as attributes and
its listeners as native events, so `@copy` on the BA screen would fire whenever a BA copied
text out of a ticket.

What shipped: the shell keeps the state, `<router-view>`'s slot renders the matched
component, and the template binds each screen explicitly (`v-if` on `route.name`, not on
`Component` — before the first navigation resolves there is no match, and `:is` on nothing
warns once per render). `AskScreen.vue` is the `<main>` that used to be inline in the shell;
the dock, the header and the diagram viewer stay in the shell because both screens or the
keyboard maths need them there.

## 3 · vue-router 5.2.0, and what it costs

Current stable, and its peers match this tree exactly (`vue ^3.5.34`, `vite ^7.3 || ^8`).
Two costs, both measured rather than assumed:

- **35 packages** in `node_modules`, because v5 ships the file-based-routing unplugin
  (chokidar, `@babel/generator`, `magic-string`, …) as runtime dependencies. None of it
  reaches the browser; `npm ci` pays for it. v4.6.4 has one dependency and would have done
  the same job — v5 was taken for being the maintained major, not for a feature used here.
  Nothing in this repo registers the Vite plugin, and nothing should until two routes stop
  being enough to hand-write.
- **The entry chunk went 371 kB → 381 kB**, and the BA screen left it: `BaScreen.vue` is a
  dynamic import, so gate + tickets + importer + the document tree are a 15 kB chunk
  (5.9 kB gzipped) a DEV never downloads. Net first paint for a DEV is about +10 kB raw.

## What else moved

- `ke.mode` → **`ke.screen`**, holding a route name. `/` resolves to whichever screen was
  last open (a BA reopening the app wants the queue), written in `router.afterEach` and read
  only by the `/` redirect record — a redirect, not a `beforeEach` that pushes, which is the
  shape that loops. A returning browser keeps a dead `ke.mode` key; nothing reads it.
- `BaScreen.vue` lost its `ask` emit — going back to the chat is a `<router-link>`. Both it
  and the header's segment use `<router-link custom>`, because `.segment` styles `> button`
  and `.btn` is written for a `<button>`: an `<a>` would arrive underlined and unstyled.
- `TestComponentsHoldNoLogic`'s positive control moved. It used App.vue's `mode` ternary to
  prove `reTernary` still matches this source; the router left the shell with no ternary, so
  the control is now the composables layer — where every branch is supposed to live anyway,
  and four files carry one.
- `vue/no-undef-components` learned `router-.*`, next to `nes-.*` and `i18n-t`. All three
  are registered outside a template's sight.

## Verified

`make check-full` green. In a real browser against the built binary (PinchTab, headless):
`/` → `#/ask`; the segment's `aria-pressed` follows the route; `#/ba` loads its chunk on
first visit and hides the dock; back returns to the ask screen; a reload at `#/ba` renders
the queue; `#/nope` redirects to the prompt; a seeded turn renders through `AskScreen`'s
props and its Copy · Regenerate · ASK BA emits reach the shell. Console clean, no uncaught
errors.
