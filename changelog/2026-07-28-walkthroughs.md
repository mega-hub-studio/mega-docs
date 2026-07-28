# 2026-07-28 — Every diagram explains itself, one node at a time

Asked for: prev/next walkthroughs that highlight and explain each step of the mermaid
diagrams. The guide's "how it works" picture already had one; the other four did not.

## The wiring is generic now

The two page-local scripts that made it work on the guide moved into `docsbase.html` and
became one pair for every page:

- **`nes-focus-svg`**, the highlight target. `<nes-walkthrough for>` is duck-typed — it
  calls `target.highlight(focus)` and does not care what the target is. `<nes-mermaid>` is
  the usual one, but its job is to *render* mermaid at runtime, which these pages
  deliberately do not ship. So this element supplies that one method against the contract
  the design system's focus CSS already reads: `.nes-focus` on the nodes, `.has-focus` on
  the view.
- **The fold**: every `<div id="X-steps">` becomes the step JSON of
  `<nes-walkthrough for="X">`, then the source markup is removed.

It has to be a **classic inline script**, and that is the one thing worth remembering:
`<nes-walkthrough>` reads its steps once, from a child JSON payload, at upgrade time — and
`elements.min.js` is a module, so it runs after the document is parsed. An inline classic
script runs *during* parse, which is the only ordering that gets the JSON in first.

Five diagrams across four pages would otherwise have been five copies of the same twenty
lines.

## The four new ones

| diagram | page | steps |
|---|---|---|
| `retrieval` | `dev.html#pipeline` | cached? · two retrievers · RRF fuse · prompt + context · stream, then store |
| `loop` | `ba.html#gap` | not in the documents · Ask BA · you answer it · it becomes a document · and then it is free |
| `specflow` | `dev.html#spec` | write the section · annotate · make check goes red · green, and published |
| `devloop` | `dev.html#local` | the fast loop · /api is real · the shipping loop |

Each step names the nodes it spotlights with `data-focus` (a `|`-separated list matched
against node text), so the picture and the prose cannot drift: a renamed node lights nothing
and the check below says so.

The BA page's steps are written for a BA — no `TOP_K`, no RRF — while the Dev page's say
why the fusion takes rank rather than score. Same diagram mechanism, different audience.

## Step titles are bilingual now

The component sets its title with `textContent`, so markup — and therefore the CSS language
toggle every other string on these pages uses — cannot reach it. Titles were English-only,
which on a page written for Vietnamese BAs is exactly the half-done bilingualism the spec
test forbids everywhere else.

So a step carries `data-title` *and* `data-title-vi`, the fold keeps both, and the language
switch rewrites `step.title` and calls `go(index)` to repaint — `steps`, `index` and `go()`
are the component's own API. 22 titles, all four pages.

## Measured, on all five

A browser check drove every walkthrough on every page at 390 and 1440: the source markup is
folded away, PREV is disabled on step 1, NEXT advances, ArrowLeft goes back, the last dot
becomes current and NEXT disables, each step lights at least one node, `.has-focus` lands on
the view, the counter reads `n / m`, and only one language is visible in the body. Then, on
the phone: the spotlit node and its explanation are on screen **together** — that is what
the diagram box's 48svh cap and `scrollIntoView({block: "nearest"})` are for.

`make check-ui` caught one thing on the way: `<nes-focus-svg>` is an unknown element, so it
is `display: inline`, and the `--flow` rhythm rule only gives a margin to a block — the
diagram box sat flush against the paragraph above it. It had been doing that on the guide
since the walkthrough first landed, and only surfaced when the wiring moved and the
preceding sibling stopped being a `<script>`.
