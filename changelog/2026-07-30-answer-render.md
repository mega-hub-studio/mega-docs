# 2026-07-30 — code in colour, evidence in pictures

Two things made an answer slower to grasp than it needed to be: a fenced block arrived
monochrome, and there was no way at all to put a screenshot in front of a dev. The fix is
mostly *wiring*, because the design system had already been built for both — that is the part
worth writing down.

## What the library already had

Read from the installed 0.14.0, not from its docs site:

| found | consequence |
|---|---|
| `<nes-code>` exists: frame, filename header, self-wiring COPY button | no component to build, and no bundle cost — it is already there |
| `components.css` defines **7** token slots; the vendor regex emits **5**. `.t-fn` (gold) and `.t-at` (cyan) are styled and never used | those two hues were reserved for a real grammar. The palette needed no new colour |
| its escaper only encodes `& < >` | `pre.textContent` is byte-for-byte the source, so an in-place re-highlight is provable rather than hopeful |
| `_done` one-shot, no `MutationObserver`, no `observedAttributes` | nothing re-renders underneath an upgrade and undoes it |
| `<nes-zoom>` already wraps the diagram viewer in `App.vue` | an image needed no second viewer |

So `<nes-code>` stayed the component and shiki became an *upgrade* to it — the vendor's regex
draws immediately, a real grammar replaces the `<pre>`'s contents when it lands, and the header
and button are never touched. An answer with no code fetches nothing; one whose language this
app carries no grammar for keeps the vendor's colours. Neither case shows an empty frame.

## Decisions

**Shiki with the JavaScript regex engine, not Oniguruma.** Measured from the built chunks:
core 34.9 KB + engine 20.0 KB + `go` 5.2 KB gzip — about **60 KB gzip for the first code
answer**, versus ~150 KB for the WASM engine alone. `forgiving: true` because the JS engine is
strict by default and *throws* on a pattern it cannot compile, which would take out a whole
answer's render over one grammar rule.

Worth correcting a number from the research that informed the plan: it estimated ~19.6 KB for
core + engine + one grammar. That was the raw npm module sizes; the *bundled* cost is ~60 KB
gzip. Still nothing in first paint, and mermaid's own precedent is 3.4 MB.

**The palette stays the app's.** `createCssVariablesTheme` emits
`style="color:var(--shiki-token-…)"` and no colour of its own; `styles.css` binds those names to
the tokens the library already uses. One correction to how the plan described this: shiki emits
inline styles, **not** the `.t-*` classes — so `.t-fn`/`.t-at` are not "lit up", their *hues*
(`--gold`, `--cyan`) are finally in use. Verified in the browser: `--shiki-token-function` is
resolving in a real answer.

**`data:` images are refused, and the reason is retrieval.** A document's body is what gets
chunked and embedded, so a base64 image would be sent to the embedding model *as text* —
hundreds of KB, poisoning the vector and paid for on every re-index. DOMPurify allows `data:`
on `<img>` by default, so the refusal had to be explicit. The alt text is left in its place
rather than dropping the node silently.

**`https:` only, remote, with the leak closed.** Render-before-upload means the image is
*always* somewhere else — that is the trade, not an oversight. `referrerpolicy="no-referrer"`
so the host learns nothing about the reader, plus `loading="lazy"` and `decoding="async"`.
Link rot is the real cost and is the reason upload is the next step.

Also worth knowing: **`<img>` already rendered before this change** — DOMPurify's defaults keep
it, and the pre-change build shows one. So this did not add images, it *constrained* them.

**Not done, deliberately:** upload, an `attachments` table, a byte-serving route. The survey is
recorded so it need not be redone: a new *table* goes in `schema.sql` and needs no migration; a
blob in `DB_PATH` is covered by `scripts/backup.sh` for free; `serveBytes` is the precedent for
serving bytes but has no dynamic content-type today.

**`<nes-annotate>` rejected.** Its published `open(index)` is broken in 0.14.0 — the
implementation is `open(i, pin)` and dereferences `pin.style` unconditionally, so calling the
documented API throws a `TypeError`. Hotspots over an image will have to wait for a release
that fixes it.

## The ordering trap, and the timing one

**`dressCode` must run after `asDiagrams`.** Both match `pre > code`. A fence turned into
`<nes-code>` first would never become a picture, and while the mermaid chunk is still loading a
graph *is* still a plain fence — so `dressCode` also skips anything `isDiagram()` recognises.
Verified: an unlabelled mermaid fence still becomes `<nes-mermaid>`.

**Where the paint is triggered from cost three attempts, and each failure was silent:**

1. `onMounted` — too early. It reads the DOM, unlike `loadFor` which reads the answer *text*,
   and at mount the screen holding the answers has not rendered.
2. `watch(() => route.name, …)` — never fires on a fresh load. `/ask` is an **eager** import, so
   the router's first navigation resolves before `App.vue`'s setup runs and `route.name` is
   already `"ask"`. There is no change to observe. It fired only when navigating to `/ba` (a
   dynamic import) and back — the shape of a bug that looks fixed while you click around.
3. `{ immediate: true, flush: 'post' }` — correct. Immediate covers the first load; `post`
   runs the callback after Vue has patched, so the elements exist to find.

## A landmine in the debugging, not in the code

While diagnosing, a probe that wrote `pre.innerHTML` from an `eval` context produced
`TypeError: Cannot read properties of null (reading 'nextSibling')` with a **Vue renderer**
stack, which read exactly like a rendering bug in the app. It was not: with error listeners
installed and no probe interference, the app throws nothing. The probe's own out-of-band DOM
write was the cause. A stack that points at framework internals is not evidence the framework
is at fault — reproducing without the instrument is what settled it, the same move that settled
the `check-ui` failure in
[`2026-07-30-deploy-from-any-tree.md`](2026-07-30-deploy-from-any-tree.md).

## Verified, in a real browser, first load and no route change

| | result |
|---|---|
| entry chunk | 410,889 B / **138,290 gzip** — up ~190 bytes gzip from baseline |
| shiki in the entry | `createOnigurumaEngine` / `vscode-textmate` / `_tokenizeWithTheme`: **0 occurrences** |
| chunks | `core-*`, `engine-javascript-*`, and one per language, all lazy |
| `go` and `sql` blocks | `data-hl="1"`, `--shiki-token-keyword/comment/function` resolving |
| `brainfuck` (no grammar) | keeps the vendor's colours, still framed, still copyable |
| unlabelled mermaid fence | still `<nes-mermaid>` |
| COPY after the upgrade | `COPY` → `COPIED!`, and `pre.textContent` still equals the source exactly |
| `https:` image | `referrerpolicy=no-referrer` · `loading=lazy` · `tabindex=0` |
| `data:` image | gone, alt text left in its place |
| a dead image URL | degrades to its alt text, rendered 646×27 — the reader still gets the description |
| Vue errors | none |

## Open

- The language label sits in `<nes-code file="…">`, a slot the library means for a filename.
  There is no other slot, and "go" above a block beats an empty header — but if the library ever
  ships a `lang` attribute, that is where this belongs.
- A dead image shows its alt text, which is honest but plain. If link rot turns out to be
  common, the answer is upload, not a nicer placeholder.
