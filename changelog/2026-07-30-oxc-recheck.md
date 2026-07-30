# 2026-07-30 — oxc re-checked, still not adopted, and one stale number fixed

Asked to integrate [oxc](https://oxc.rs) for DX/ROI. The answer is no, and it was already no —
`web/ui/eslint.config.js` has argued against oxlint since the SFCs landed. What this entry adds
is a **dated re-check**, a **stronger citation**, and a **trigger that can be tested**, because
"we looked at this once" decays into "nobody remembers why".

## Three reasons, in the order they matter

**1. oxlint's blind spot is this repo's most-enforced rule.** It parses no `<template>` at all.
Not "partially" and not "experimental" — upstream says it plainly:

- `oxc.rs/compatibility.html`: *"Vue, Svelte, Angular, Ember, Nuxt, Astro, SvelteKit, and
  Analog: **No template linting yet**"*
- `oxc.rs/docs/guide/usage/linter.html`: framework files are handled *"by linting only their
  `<script>` blocks"*

oxlint does ship ~39 `vue/*` rules, and every one of them runs on the script AST — deprecated
APIs, props declarations, lifecycle misuse. Searched its 847 rules: no `no-undef-properties`
equivalent exists.

Measured here, `--print-config src/App.vue`: **149 of 161 `vue/*` rules are on**, and
`vue/no-undef-properties`, `vue/no-undef-components` and `vue/require-explicit-emits` are all at
`error`. Those three are the named enforcers of CLAUDE.md rules 11 and 12 — both of which are
*about templates*. A fast linter that cannot see templates would report green on exactly the
thing this gate exists to catch, which is worse than being slow.

**2. oxfmt reverses a recorded decision.** It is Prettier by behaviour — *"passes 100% of
Prettier's JavaScript and TypeScript conformance tests"* — so it reprints from the AST and
reflows. `eslint.config.js` chose eslint-stylistic over Prettier precisely because it *"reflows
nothing — only semicolons, quotes and trailing commas change"*. Adopting oxfmt means reflowing
all of `web/ui/src` in one diff. And for `.vue` it is not even native: the formatter's
language-support page puts Vue in a *"Prettier-backed"* category, delegating everything but the
`<script>` block to a bundled real Prettier.

**3. The speed is real and is not the expensive part.** ~4s → ~0.1s for the script half, while
ESLint still has to run for the other 149 rules — so the *gate* barely moves; only the
edit-loop does. And `make check` is dominated by `go test` plus `golangci-lint`, not by ESLint.
Rule 20's question — what breaks today without it — answers itself.

## What was actually wrong, and is now fixed

The paragraph making this argument ended with a measurement, and the measurement had gone stale:

| | files | lines | ESLint warm | cold |
|---|---|---|---|---|
| when written | 28 | — | ~2000 ms | — |
| **2026-07-30** | **46** | **6009** | **~3600–4800 ms** | **~7300 ms** |

The tree doubled and the cost doubled. A stale number inside the sentence that justifies a
decision is the rule-24 failure mode in miniature: the conclusion still held, but the evidence
for it no longer did, and nothing would have caught that. The comment now carries the date, the
counts, and the command to re-measure.

## oxc is already here — at the layer that wanted it

Worth writing down so nobody reads "integrate oxc" as unfinished work. `vite@8.1.5` pulls
`rolldown@1.1.5`, which is Oxc's flagship consumer (*"It powers Rolldown (Vite 8's bundler)"* —
`what-is-oxc.html`; `oxc-minify` is Rolldown's default minifier). Installed today:

```
rolldown@1.1.5              + the @rolldown/binding-* platform set
@oxc-project/types@0.139.0  ← type definitions only
```

No `oxc-parser`, no `oxc-transform`, **no `oxlint`** (`grep -c oxlint package-lock.json` → 0).
So the bundler half of oxc is in the tree, doing the parse/transform/minify work in `make ui`,
with no config of ours and nothing to decide. The linter and formatter are separate tools and
would each be a net-new dependency.

## Trigger to re-check — testable, not a reminder

Revisit when **either** of these becomes true:

1. oxlint ships `vue/no-undef-properties`, `vue/no-undef-components` and
   `vue/require-explicit-emits` — the three by name, since they are what rules 11 and 12 point at.
2. `oxc.rs/compatibility.html` no longer carries the line *"No template linting yet"* for Vue.

Then re-measure with the command in `eslint.config.js` and compare against the table above.

This lives here and not as a `TODO`/`ponytail:` comment in the code on purpose: rule 24 makes a
deferred marker a lint error in both languages (`godox`, `no-warning-comments` — the latter is at
`error` here, verified), and CLAUDE.md's *deltas* table already records this exact case against
ponytail's own convention.

## State outside git

None. No dependency added, no config file added, `formatters: false` untouched, `Makefile`
untouched.
