# 2026-07-27 — The front end is Vue 3.5 Composition API, one composable per concern

`app.js` was a 376-line Options API object holding seven unrelated concerns in one
`data()`: the conversation, the corpus, the scope, the ticket queue, health and prices,
the status line, and the lazy diagram renderer. Any of them could collide with any other
by name, and the file had to be read whole to change one of them.

It is now wiring, and each concern is a composable in `web/app/use/`:

| file | owns |
|---|---|
| `conversation.js` | turns, ask/regenerate/stop/reset/copy, streaming, session persistence, the `busy` attribute the prompt element needs |
| `corpus.js` | what is indexed, and the starters derived from it |
| `scope.js` | which folder answers, and its one storage key |
| `qaloop.js` | the ticket queue and the free-to-replay history — shell state, because both screens render it |
| `runtime.js` | online · writes · model and prices, all from one `/api/health` |
| `statusline.js` | the bottom strip, as one computed object |
| `diagrams.js` | the 3.4 MB renderer arriving late, and the zoom viewer |

`ba.js` and `tree.js` moved to `setup()` too, so there is one idiom in the front end
rather than two. Template contracts are unchanged: `index.html` was not touched except to
preload the new modules.

## The two rules that keep it honest

- **A composable never reaches for another's state.** What it needs arrives as an
  argument. `useConversation` gets the scope, a scroll function and an `onSettled`
  callback — not the corpus, not the renderer. The shell is the only place that knows the
  whole picture, which is what makes each file readable alone.
- **Everything the template names must be in the returned object.** A missing key is
  `undefined` at render with no error. Before writing a line of this I enumerated every
  identifier the two in-DOM templates bind — 49 in the shell, 47 in the BA screen — and
  checked the returns against that list.

Vue 3.5 specifics used: `useTemplateRef()` instead of `this.$refs` for the five elements
the app touches directly (the prompt, the dock, the scope `<details>`, the viewer
`<dialog>` and its body). Composables read `Vue` *inside* the function, never at module
scope — the global comes from a classic script, and a module body can evaluate first.

## Still no build step

This is the migration that does not change how the thing ships: no bundler, no
`node_modules`, no SFC compile, `make vendor`'s air-gapped story untouched, and the Go
binary still serves `web/app/` straight from its embed FS. The Vue global build ships the
compiler, so in-DOM templates and string templates cost nothing at runtime.

**What a Vite + SFC step would add**, if it is ever wanted: real `.vue` files with
scoped styles and compiler macros (`defineProps` destructuring, `defineModel`), and
type-checking through `vue-tsc`. What it would cost: Node on the build host and in CI, a
`dist/` step before `make build`, the SRI-pinned CDN story replaced by bundled output, and
the "one Go binary, no Node" line deleted from three documents. The composable split above
is the part of that migration worth having on its own — and it is the part that makes the
rest mechanical if the trade is ever taken.

## Verified in a real browser (390px and 1440px)

Eleven steps, all passing: empty state with corpus-derived starters · the scope picker in
the dock · ask streams an answer with citations · the `busy` attribute clears · the status
line reports tokens and time · a repeat comes back **CACHED** · a folder picked from the
tree mid-thread (picker closes, storage set) · the scoped answer badged and cited only
from that subtree · **Ask BA** files a ticket and badges the turn · reset clears the thread
· BA unlock → confirm publishes into the corpus.

Plus, separately: diagrams still fit in the card and open at natural size (16px labels,
correct theming, one id each), and a reload restores the thread, the scope *and* the
rendered diagram with no stuck spinner. No console errors — the single 400 in the log is
`upload.verify()`, which probes the password with an empty multipart POST on purpose.

One correction to my own tooling: the boot smoke script had been reporting FAIL since the
BA screen moved into a `<template>`, because it looked for `{{` in `body.innerHTML` and an
inert template legitimately contains them. It now strips templates first — a false alarm
that had already cost me one investigation.
