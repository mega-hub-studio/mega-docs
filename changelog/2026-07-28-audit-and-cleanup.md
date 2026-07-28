# 2026-07-28 — Auditing the first screen, and what a dead-code sweep actually found

Two passes, both asked for: four cleanup reviews over the previous commit (reuse,
simplification, efficiency, altitude), then a dead/redundant-code sweep across the whole
front end. The second pass is the more interesting entry, because most of what it looked for
was not there.

## The audit found three library semantics I had assumed rather than read

All three are in the previous commit, and all three are the same mistake in different
clothes — reading a class name as if it described a *shape* when in this design system it
describes *content* or *severity*.

- **`.callout crit` is not a modifier the library ships.** The set is
  tip/gotcha/memo/quest/info/warn; `crit` is a `.badge` fill. So it fell through to the
  default gold and rendered "can't read the index" in almost the tone of the milder `warn`
  one branch above — inverting the severity of the two states the pair exists to
  distinguish. Three of the four reviews found it independently.
- **`.palette-list` is nested inside `.palette`.** The history list's copy of that class
  matched nothing, so those rows silently lost the 2px rhythm and inset that a comment
  claimed they shared with the menu. They have the app's own `.results` stack now; `.result`
  itself is top-level in the library, so the rows are still the library's.
- **`.result-hint` was styled unscoped** — a bare library class, which is the exact
  ambiguity `.row` already taught this repo about.

Two of them were invisible to every measurement I had taken, because geometry probes read
computed values and all three rendered *something*. The screenshot caught one; reading the
library's CSS caught the other two. Worth remembering: a numeric check confirms the layout
it was told to look at, and says nothing about whether a class means what you thought.

## The efficiency review refuted the thing I suspected and found a real one

I went in expecting the per-keystroke re-sort in `rankDocs` to be the cost. Measured: **8.6µs
at the server's own 100-document ceiling** — under 0.05% of a frame. Left alone; splitting it
into two chained computeds would have bought nothing and cost a computed plus an export.

The real cost was somewhere I had not looked. Adding a filter field gave the component
reactive state for the first time, so **every keystroke now re-evaluates every template
expression in it** — including `shortDate()` on each ticket in a *collapsed* disclosure.
`toLocaleDateString` rebuilds a formatter per call: **78µs against 2.0µs** through one
hoisted `Intl.DateTimeFormat`, i.e. 5.6ms of avoidable work per keystroke at 100 tickets.
One module-scope constant fixes it, and fixes `TicketCard`'s two call sites for free.

Also removed: the history rows carried the same `refresh` glyph on every row — no
information, and `<nes-icon>` assigns `innerHTML` in `connectedCallback`, so that was 20
element upgrades and 6.3 kB of SVG parsed inside a collapsed disclosure.

## The docs were the worst of it

These pages are the spec, and the previous commit left three passages in `docs.html`
describing the feature it deleted — the retrieval tip, the feature-table row ("Start from a
suggestion"), the first-run checklist — in both languages. Rule 15's join covers routes and
env vars, so `make check` stayed green while the prose described something the code no longer
does.

Worse, `AGENTS.md` still promised *"there are currently no local overrides of the design
system"* while that commit added one. It now names the override, the version that lacks the
fix, and the condition for deleting it — plus the line the rule actually needs:
`align-self` on a child is **placement** (the app's job); `max-block-size` on a component is
an **override** (the library's).

## The dead-code sweep: four surfaces, one finding

This is the part worth writing down, because "we looked and it was clean" is a result and
the next person should not have to redo it.

| surface | how it was checked | found |
|---|---|---|
| Go, whole-program | `deadcode` from the two mains | **0 unreachable** |
| JS exports (`lib/` + `composables/`) | every exported symbol grepped outside its own file | **1**: `treeNodes` |
| app CSS (`styles.css`) | all 16 class selectors against every `.vue`/`.js` | 0 unused |
| docs-page CSS (`docsbase.html`) | all 20 selectors against four rendered pages | 0 unused |

The one finding: `treeNodes` was exported but only ever called inside `nestree.js`. Now
private.

Two things to record about the method, since both nearly produced false work:

- **`deadcode` had been silently skipping all session.** The Makefile prints
  `skipped deadcode (go install …)` when the tool is missing, and it was — so every green
  `make check` this session carried no reachability check at all. Installed, it reports zero.
  The skip message is doing its job; I was not reading it.
- **My first CSS scan reported two dead selectors, and both were comments.** `.md` came from
  the string "DESIGN.md" and `.wt-dot` from a comment explaining that *that very override had
  already been removed* in 0.7.1. Strip comments before treating a regex match as a selector,
  or a dead-code sweep invents work.

## What was deliberately not done

Three extractions were considered and rejected, each because it would move code rather than
remove it. Recorded so they are not re-litigated:

- **A `StatusBadge.vue`** for the `STATUS[x.status]` badge repeated in three components. The
  commonization that matters already exists — `STATUS` in `lib/qa.js` maps a status to its
  class and label. What is left at each site is one line of markup, and all three still need
  `STATUS` for the accent or the hint, so the import would not even go away.
- **A `ResultRow.vue`** for the two `.result` lists. They share four static wrapper elements;
  the icon, both text sources, the trailing hint, the tooltip and the emit all differ. It
  would need five props and put a `v-if` on one of them — the shape rule 11 exists to keep
  out.
- **`shallowRef` for `runtime` and `progress`.** The Vue skill's advice is right in general,
  but those two hold flat objects of primitives: there is nothing nested to proxy, so it is
  one Proxy either way. Churn with no gain.

`shallowRef` *was* applied where it buys something — `corpus`, `queue` and `history` hold up
to 100, 100 and 20 nested objects respectively, and every one of them is replaced wholesale
and never edited in place (checked before changing, since a wrong `shallowRef` fails silently
and this repo's own trap list already has one entry of that kind). `turns` deliberately stays
a deep `ref`: streaming does `turn.a += tok`, so the deep reactivity is load-bearing.

Verified after, against the real binary and the seven-document fixture: seven rows at one
width and one gap on both viewports, the filter narrowing to four and recovering to seven,
the history list rendering with the menu's rhythm and zero icons, and a tap still streaming a
cited answer. `make check` (now including `deadcode`), `check-ui`, ESLint and golangci-lint
2.12.2 all clean.

## Still open

- The host is still not redeployed; `/opt/knowledge/corpus` still has no remote.
- **The fake provider is still not committed** — it is now the fourth session it has been
  rewritten in, and every measurement in this entry and the last two depends on it.
- Reported upstream to 8bit-nes, none blocking: an in-page variant of `.palette` (drop the
  modal cap and `--sh-5`), a mono/tabular default for `.result-hint`, and `.callout.crit` as
  an alias for `gotcha` so the accent vocabulary stops being a trap.
- Declined pending the owner's call: moving the ready state out of `.empty` into a `.card`.
  `.empty` is documented as the no-data panel, and three app rules exist only to undo its
  centring — the deeper fix deletes them, but it changes how the screen looks.
