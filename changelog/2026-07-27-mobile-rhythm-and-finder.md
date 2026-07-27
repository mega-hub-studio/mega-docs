# 2026-07-27 — What a screenshot shows, measured

Two photographs of the published guide on an iPhone, and both showed layout the browser
checks had passed. Worth writing down, because the reason they passed is the interesting
part: every assertion was about *the page* (no sideways scroll, wide content scrolls in its
own box, tap targets 44px) and none was about *a box being wide enough to hold a word*.

## What was wrong

| what the phone showed | cause |
|---|---|
| "Start in 60 seconds" as **uppercase mono columns four characters wide** | `<ol class="steps">`. In 8bit-nes `.steps` is a horizontal stage bar — `inline-flex`, mono, uppercase, `--dim` — for "STAGE 1 · 2 · 3". Used for a numbered list with paragraphs in it, that is what you get. Three of these had been written (guide ×2, BA page ×1); they are `.step` blocks now, the recipe the rest of the pages already used |
| the three-column "If it doesn't open" table with **one word per line** | `.table th` is `white-space: nowrap`, so the row header takes its natural width and the two value columns split what is left — about four characters each at 390px |
| blocks **touching**, and gaps of 0 · 12 · 16 · 24px in one scroll | every recipe brought its own margin (`.table-wrap` sp-3, `.step` sp-4, `.callout + .callout` sp-3, the terminal inside a step sp-2) and anything with no rule of its own got nothing. Measured: **31 pairs of blocks at 0px** on one page |
| the second row of the header **empty** except the language toggle pushed right | nothing was in it |

## What it is now

- **One rhythm, declared once.** `--flow` (`sp-4`) for every block that follows another —
  in the article, inside a step's body, inside a tab panel, inside a card — plus `sp-5`
  above an `h3` and `sp-7` between sections. Every per-recipe margin is gone. Measured
  after: **three gap values, zero touching pairs**, on all four pages in both languages.
  One subtlety worth the comment it has: both languages are in the DOM and a hidden
  sibling still counts for `+`, so the *visible* first block of a container is its second
  child half the time — the first pair is zeroed, or a Vietnamese reader gets a 16px dent
  at the top of every tab panel.
- **A table on a phone is a list of rows, not a grid.** Below 40rem each row is a block:
  the row header is its title, each value is a labelled line under it, nothing scrolls in
  either axis. The labels are the column headings, copied onto the cells by the foot script
  from the `thead` row of each language — they are already on the page, so they are not
  typed a second time. With JS off the values still read in order.
- **No column narrower than 14ch above 40rem.** The Dev page's HTTP table had a 17ch
  column ("never, so probes need no secret" over three lines). A floor makes the table
  wider than its wrapper instead, and `.table-wrap` already scrolls inside its own frame.
- **The header's second row holds a section finder.** `<nes-input-menu>` built from
  `<nes-toc>`'s own rows — so it lists exactly the sections that exist, with the ids the
  index already resolved, in the reading language. Type, pick, jump. Rebuilt on every
  language change (the component reads its options once from a child JSON payload, the
  same contract as `<nes-tree>`), and hidden entirely when a page has no index.

One bug found while building it: the popup opened **behind** the sticky index bar —
`nes-toc` sets `z-index: 20` on itself and the header was at `10`. A popup you can see and
cannot tap, with no error anywhere. The header is at `30` now, still under the design
system's `--z-overlay`. Its own z-index could never have fixed it: it is inside the
header's stacking context.

## The check that would have caught all of it

`make check-ui` (new): renders the guide, serves it, and measures it in Chromium at 390 and
1440, in both languages, over all four pages. It skips politely when Playwright is not
installed — a browser is a tool here, not a dependency, and rule 14 still says this product
needs no Node.

On top of what it already checked, it now fails on:

- a table cell or step body **under 18 characters wide**
- **any two blocks with less than 8px between them**, or more than three distinct gaps on a
  page
- an `<li>` under `.steps` — the recipe collision above
- table rows not stacked (and not labelled) on a phone, or stacked on a laptop
- the finder missing, under 44px on a phone, on a different row from the language toggle,
  in the wrong language, or **below `nes-toc` in the stacking order**

Screenshots are how these were found; measurements are how they stay fixed.

## Deploying this

Docs only — no Go change, no schema change, no re-ingest. The published pages update on the
next push to `main`; the app binary is unaffected.
