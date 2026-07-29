# 2026-07-29 — 8bit-nes 0.13.0, and an answer's diagram explains itself

Two things, and the second is only cheap because of the first: the design system goes
0.8.0 → **0.13.0**, and the walkthrough the guide's five static diagrams have had since
`2026-07-28-walkthroughs.md` now works on the diagrams a *model* draws.

## The bump was already half-applied, and the half that was missing breaks the guide

`web/vendor.sha384` said `8bit-nes@0.13.0` with **0.8.0's digests** for `all.min.css` and
`elements.min.js`. That is the failure the file's own header warns about, arrived: the
published pages render `integrity="sha384-YjUQ…"` against bytes whose digest is
`sha384-f74t…`, so the browser refuses the stylesheet and the guide loads with no design
system at all. `make vendor` refuses the same mismatch, which is how it surfaced.

Both digests are corrected from the package's own `sri.json` and then re-derived
independently by `make vendor` (it fetches the registry tarball and will not write a file
whose digest disagrees). The three woff2 faces are **byte-identical to 0.8.0** — only the
CSS and the element bundle moved.

`web/ui/package.json` had not been touched at all, so the app was still bundling 0.8.0
while the docs pages claimed 0.13.0 — the two-manifest split of rule 7 working as intended,
and both are on 0.13.0 now. `web/dist/build.json` picks the version up from the manifest
via `stamp.js`, so the startup line needed no edit:

```
mega-docs v0.13.0 on http://127.0.0.1:8080 (ui: vue 3.5.40 · 8bit-nes 0.13.0 · build …)
```

## Nothing upstream let a local override go, and that was checked against bytes

The three rules in `web/ui/src/styles.css` are all still needed. Read out of 0.13.0's own
`components.css` rather than its changelog, because "fixed upstream" is a claim about bytes:

| override | 0.13.0 | |
|---|---|---|
| `.palette-list` un-capped | `max-block-size: min(50vh, 340px)` + `overflow-y: auto` | still sized for a modal |
| `.prose a.cite` cyan, no underline | `.prose & a { text-decoration: underline }`, unscoped | still outscores `.cite` |
| `.source-title:hover` un-underlined | still underlines | nothing upstream to fix |

So AGENTS.md's table keeps all three and only its version moved. What 0.9.0–0.13.0 *did*
bring that this repo can use later, none of it wired up on its own authority (rule 20):
`.toolbar`, `<nes-split>`, `<nes-popover>`, `confirmDialog()`, and a `--bp-2xl` (1600px)
rung that steps the type scale one notch — which is why `make check-ui` at 390 and 1440 is
unchanged, and why nothing here was retuned.

The **toast XSS fix** in 0.12.0 arrives with the bump: `toast(msg)` rendered `msg` as HTML,
so `toast(userInput)` was an injection. This app passes only its own strings, so it was
never reachable — but the fixed bundle is the one shipping now.

## The walkthrough, and the one line the library does not carry

`<nes-mermaid>` **already ships `highlight(focus)`** and re-applies it after every draw
(`_reapplyFocus`), and `<nes-walkthrough for>` calls exactly that on whatever `for` names.
So the app needed no highlight code, no component, and no shim — the guide's
`<nes-focus-svg>` exists only because its diagrams are build-time SVGs with no mermaid at
runtime, and that shim stays for that reason and no other.

What the app needed was the *fold*, in `lib/answer.js` beside `dressTables` and
`dressTaskLists`, because it is the same kind of decision: what the answer's HTML may
contain. The prompt already said **"the full wording belongs in the prose underneath"**;
that sentence now says *how* — a numbered list in the diagram's own order, one item per
node, each opening with that node's exact label in bold. It is a shaping of a rule that was
already earning its tokens, not a new one, and a model that ignores it costs nothing: no
`<ol>`, no walkthrough, the diagram exactly as before.

Three things the code decides rather than trusts:

- **It is a fold, not an addition.** The list becomes the stepper and the original is
  removed, or the same prose sits on the page twice. Only a diagram that really became a
  drawing is folded — while the 3.4 MB renderer loads, the fence and its list stay
  together, because a fence with no explanation under it is worse than either.
- **The decision comes before the mutation.** A list where one item has no bold lead is not
  a node walk, and half a fold would leave a stepper with a hole in it. So it is read into
  steps first and applied second.
- **`</script` cannot survive in the payload.** The steps ride in a
  `<script type="application/json">` child, `<script>` is a raw-text element, and the
  serializer does not escape what is inside one. DOMPurify cannot leave a `</script` behind,
  and `<` is escaped anyway rather than resting on that.

### The library highlights; it does not scroll

Measured at 390×844, and it is the reason `diagram.onStep` exists: the box is capped at
48svh (405px), a seven-node graph draws 554px, and `_reapplyFocus` toggles the classes and
scrolls nothing — so the last two steps lit a node 150px below the fold and the reader saw
the sentence change and the picture not. The guide's shim had carried that
`scrollIntoView({ block: "nearest" })` all along; the app gets it on the library's own
bubbling `nes:step`, wired exactly like `nes:render` already was (one listener on the
answer, because a walkthrough arrives inside `v-html` and Vue never sees it).

`nearest` and not `center`: the diagram box scrolls, the page never does. Verified — the
page stayed at 0 while the box scrolled 161px.

## Measured, through the front door

The app has no browser check of its own (rule 21: the running product is the verification),
so this was driven end to end against a **stubbed `/api/chat` SSE stream** — the app's own
transport boundary, the app's own prompt box, the component's own dots. Two token frames so
the streaming path is the one under test, at 390×844:

7 nodes drawn · `data-walk` set · `for` resolves to the diagram · 7 dots · `1 / 7` ·
the source `<ol>` gone · **1 citation marker and 1 source row still there** · the
`⤢ TAP TO ZOOM` affordance still there · cap 405.12px · step 1 lights `Client` · the last
dot lights `Archive`, **inside** the capped box, box scrolled 161px, page unmoved.

`make check-full` is green, including `check-wt` — the guide's five walkthroughs driven
prev/next/ArrowLeft at 390 and 1440 on the new `elements.min.js`, 11 assertions each, every
step still lighting at least one node. That is the evidence the highlight contract survived
five minor releases.

## Two stale claims retired on the way

`retiredClaims` gained a pair, both halves of one sentence on the Dev page: it described a
zoom viewer that read the `viewBox` and **pinned a width in JS**, which is code `<nes-zoom>`
replaced — `diagram.js`'s own header says "nothing here sizes the copy". A correctly spelled
paragraph describing deleted code is exactly what rule 26 is for, and it had been green the
whole time.

The Guide page gained a **Walk through a diagram** row, both languages, in the same commit.

## Housekeeping, said plainly

- `ChatTurn.vue` and `AdminScreen.vue` had been reformatted by **Prettier** — semicolons,
  double quotes, `</span\n>` — which is 26 errors against this repo's ESLint config and
  therefore a red `make check` that had nothing to do with anyone's work. Fixed with
  `eslint --fix` (formatting only, no semantic change). Worth a look at the editor
  setting that did it, or it returns on the next save.
- `web/ui`'s 7 high-severity audit findings are all **devDependencies** (the eslint →
  minimatch → brace-expansion DoS chain), pre-existing and unrelated to this bump. Nothing
  in them reaches a browser. Left alone: a dependency bump is its own change.

## Still open

Carried over, unchanged by this entry:

- **The host has not been redeployed** since the Vite migration.
  `cd /opt/knowledge && git pull origin main && make build && sudo systemctl restart knowledge`.
  This bump changes the published guide's digests too, so a stale cached `elements.min.js`
  cannot be served under the new page — it would fail SRI rather than run.
- **A fake provider is still not committed.** This session wrote its third throwaway one, as
  a `fetch` stub in a scratch probe rather than a file. That is now three times, which is
  three times the signal. Acceptance: `make smoke` runs with no key and no cost.
