# 2026-07-27 — Components are headless; the logic all lives in composables

The composable split earlier today moved the *shell's* state out of `app.js`. It left the
BA screen as it was: a 179-line component whose `setup()` still held the password gate,
the import pipeline and the ticket transitions. So the rule was half-applied — logic in
`use/` for the shell, logic in the component for the screen.

Now it holds everywhere, and the architecture is four layers with one sentence each:

| layer | files | may contain | may not |
|---|---|---|---|
| **plumbing** | `chat.js` `qa.js` `upload.js` `answer.js` `diagram.js` `library.js` `session.js` `viewport.js` | fetch, SSE, storage, markdown, DOM maths | any Vue import — these run in a bare console |
| **logic** | `use/*.js` | reactive state and every branch | another composable's state, or markup |
| **components** | `ba.js` `tree.js` | props, emits, compose, return | branches — a component with an `if` is a composable nobody wrote yet |
| **wiring** | `app.js` | who gets what, and what the template binds | logic of its own |

## What moved

| new composable | took over |
|---|---|
| `use/gate.js` | the BA password: unlock (checked *before* it is stored), and what a refused write means — a wrong password says so in the form and goes back to locked; a 403 is writes-disabled and must not read as a typo |
| `use/importer.js` | files, the real progress counts, partial success, the drop-target state |
| `use/tickets.js` | draft · confirm · reject through one path, plus the drafts being typed |
| `use/nestree.js` | `treeNodes()` and the rebuild rules for `<nes-tree>` |
| `use/toast.js` | the injected design-system helper, with a no-op default so a component mounted without a provider degrades instead of throwing |

| component | before | after |
|---|---|---|
| `ba.js` | 179 | **54** — four props, two emits, three composables, one return |
| `tree.js` | 110 | **33** — a host element and one call |

`app.js` stays at 202 lines of wiring. Nothing in `index.html` changed except the preload
list: the template contracts are identical, which is the point — this is a rearrangement of
where behaviour lives, not a change to what it does.

## The detail worth keeping

**Reactive inputs a composable does not own arrive as getters**, not values:
`useImporter({ documents: () => props.documents })`, `useTickets({ tickets: () => props.queue.tickets })`.
Passing the array would freeze the composable on the array that existed at setup, and the
shell replaces the corpus wholesale after every import — the folder suggestions would have
quietly gone stale. The getter is also what `watch` needs to see a prop change at all.

`ba.js` returns `{ ...gate, ...tickets, ...importer }`. That is deliberate and only safe
because the three own disjoint names — which is easy to check now that each is one screenful,
and was not when they shared one `data()` object.

## Verified

The eleven-step browser pass again, at 390px: empty state and starters · the dock's scope
picker · streaming with citations · `busy` clearing · the status line · a **CACHED** repeat ·
a folder picked mid-thread · the scoped answer badged and cited from that subtree only ·
**Ask BA** · reset · BA unlock → confirm.

Plus the import path specifically, because `useImporter` is new code: all three drag events
`defaultPrevented`, a two-file drop indexing `pricing.md — 1 sections` and naming
`skipme.pdf — not .md,.markdown,.txt`, and the shell's corpus count moving after the
`changed` event. No console errors; the one 400 in the log is `upload.verify()` probing the
password with an empty multipart POST on purpose.
