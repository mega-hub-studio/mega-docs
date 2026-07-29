# 2026-07-29 — The BA library was four classes that match nothing, and the dock's strip was 12px short

Reported from a phone, as four screenshots: the library list in two ragged columns with paths
broken every six characters, the status strip ending short of the right edge while the left one
bled off, a truncation notice floating centred under left-aligned rows, and a selected word
lit up like a flare. Every one of them had a cause, and none of the causes was spacing.

## The library list: `.datalist` + `.row` + `.grow` + `.hint`, none of them doing what the markup assumed

`LibraryPanel.vue` rendered nine sibling `<div class="row">` inside `<div class="datalist">`.
Measured against the built CSS, that is four separate misses:

| class | what the markup expected | what 8bit-nes 0.13.0 defines |
|---|---|---|
| `.datalist` | a row list | a **description** list: `display: grid`, `grid-template-columns: auto 1fr`, rules for `> dt` / `> dd` |
| `.row` | a flex row | `.tree .row` and `.control-group.row` — nothing bare |
| `.grow` | flex: 1 | only this app's `.bar .grow` |
| `.hint` | secondary text | only `.field > .hint` |

So document 1 went into grid column one, document 2 into column two, and each path was laid
out in half a phone with `overflow-wrap: anywhere` doing the rest. The `@container
(width<=320px)` single-column fallback the library ships could not save it either: nothing in
this app declares `container-type`, and a container query with no container never matches.

A fifth was in the same rows: `.perm` on the REMOVE button. It is a confirmation **block**
recipe — `display: flex`, `inline-size: 100%`, panel background, 6px warn edge — not a button
modifier, so it turned a 40px button into a full-width card. And it confirmed nothing:
`useLibrary.drop()` removes on the press it receives. A mis-tap on a phone took a document out
of the knowledge base, and the comment above the button claimed the opposite.

**What it is now:** the same `.result` row the ASK screen's document menu already uses —
`.result-title` and `.result-path`, both ellipsised, so a 90-character path cannot re-shape a
row — with the count and the date in fixed-width columns and the two actions as `edit` /
`trash` icon buttons on the same line. Measured at 390×844: one row height (60px), one x for
the title (50), the count (197), the date (297) and the actions (371) on every row. A document
now looks like a document on both screens, which was the other half of the complaint.

REMOVE arms first (`armed` in the composable) and says `SURE?` in `--crit` before it acts.

Two smaller things in the same file: `docTitle(d.path)` was passing a string to a function that
takes the document, so it returned `""` and any document without its own title had an empty
row; and `.badge clear` was on the count and the kind, where `clear` is this library's
*green/good* fill — a taxonomy label claiming a pass state, which is the mistake
`EmptyScreen.vue` already had a comment about.

## The strip that was 12px short: `inline-size: 100%` against a negative margin

`.dock .statusline` bleeds out of the dock's padding with `margin-inline: -12px`. The recipe
sets `inline-size: 100%`, which resolves against the dock's *content* box — so the strip was
the padded width while starting 12px to the left of it: flush past the left edge, 12px short of
the right. That is the asymmetry you see before you can name it. `inline-size: auto` lets a
block-level flex box fill its container plus both negative margins, which is the whole trick.
Measured after: the strip and the dock both run 0 → 390.

Its second-line problem was `.sl-end { margin-inline-start: auto }`. On a phone the strip
wraps, and an auto margin on the wrapped group meant line one stopped early while line two sat
flush right — two lines sharing no edge. Below 640px the push is off, so items fill the line
before breaking and both lines start at 12px; at 640px and up it comes back, where nothing
wraps and the strip is a bar with two ends again.

## Less chrome while reading: the bar scrolls away on a phone

The bar (60px) and the dock (150px) were a quarter of an 884px window, so an answer was read
through a 500px slot. The bar is navigation — which screen, which language, start over — and
none of it is needed *while reading*, while the thing that is (the prompt) is in the dock and
stays. So below 640px it is not sticky: it scrolls off, and one flick to the top returns it.

**Rejected, on purpose:** hide-on-scroll-down / show-on-scroll-up. It needs a scroll listener,
a composable and a hysteresis threshold to not flicker, and it buys the same 60px that one
declaration already buys. If it is ever wanted, the reason will be that the mode switch is
needed mid-read — which would be a fact about how the app is used, not a preference.

## Two more alignment fixes

- The document menu's truncation notice ("N more match — type to narrow") is a
  `.palette-empty`, which the library centres because its own case is a *state* filling a blank
  list. Under nine left-aligned rows it hung off both their edges and broke mid-sentence.
  Start-aligned now, at the inset a row's text actually has: `--sp-2` (the list) + `--bw-2`
  (`.result`'s transparent hover edge) + `--sp-3` (its padding) = 22px. Measured: the notice's
  first character and the row's are both at 25px inside the palette.
- `::selection` was a solid `--primary` fill with near-black text: about 11:1 against this
  page, which is right for a light theme and a flare on a dark one — a long-press on a phone
  lit the whole word. A 32% tint of the same colour with `--ink` on top marks the range and
  stays readable.

## How this was verified, and what it cannot tell you

`make check-ui` measures the *guide pages*, not the app, so there is no rig for this. What was
used instead: a probe page under `web/dist` loading the built bundle plus the library's
`elements.min.js` (for `<nes-icon>`), driven at a real 390×844 viewport —
`pinchtab set viewport 390 844` — reading `getBoundingClientRect()` for every column edge. Two
traps that cost time and will again:

1. **`make ui` empties `web/dist`.** A probe file there disappears on the next build; put it
   back rather than wondering where it went, and delete it before the last build.
2. **A 390px-wide `<div>` is not a 390px viewport.** Media queries read the viewport, so the
   first run showed desktop behaviour inside a phone-width box and the `.sl-end` fix looked
   like it had not applied.

Every recipe claim above was re-read from `web/ui/node_modules/8bit-nes/all.min.css` at
**0.13.0** after the pin moved mid-session, not from 0.8.0 where the work started. `.datalist`,
`.perm`, `.result`, `::selection`, `.statusline`, `.sl-end`, bare `.row`, absent `.grow` and
the `.prose a` / `.cite` collision are all unchanged between the two.

## Open, for upstream

Both would delete a local rule, and neither blocks anything here:

1. Scope the prose-link rule (`.prose a:not(.cite)`, or move `.cite` to a later layer) — the
   same collision hits `.chip`, `.badge` and `.kbd`, every single-class recipe that can appear
   inside `.prose`.
2. `.source-title`'s hover underline belongs on `a.source-title`, so a read-only list does not
   advertise a link.

And one for this repo — **decided against, so nobody spends a day on it.** A class that matches
nothing fails silently, `vue/no-undef-properties` covers bindings and not classes, and the
obvious answer is a join between the class names in `web/ui/src/**/*.vue` and the selectors in
the built CSS. It was drafted, then dropped on the arithmetic: of the five classes that broke
this screen it would have caught **two**.

| class | a name-existence check sees |
|---|---|
| `.row`, `.grow` | **caught** — no selector anywhere |
| `.hint` | passes: it exists, as `.field > .hint`, which is not where it was used |
| `.datalist` | passes: it exists, and is a grid for `dt`/`dd` |
| `.perm` | passes: it exists, as a confirmation *block* recipe on a button |

Three of five are "defined, and wrong for this context" — invisible to any name match, and the
check would still need an allowlist for `data-accent`, runtime state classes and the classes
`<nes-*>` builds in `connectedCallback`. A gate that catches 40% and can go red on a false
positive is worse than the habit it replaces, which is now in AGENTS.md: **grep the class in
the built CSS and read the rule you find.**
