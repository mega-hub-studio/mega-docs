# 2026-07-27 — Critical rules, each one naming what enforces it

The architecture was documented this morning and refactored twice this afternoon. Prose
does not hold a refactor in place, so the rules are now checks — and the ones that cannot
be checked say so.

## The table

`CLAUDE.md` opens with fourteen rules and, beside each, the test or target that fails when
it is broken. Ten of them already had an enforcer. **Four are new**, and they exist because
the Vue 3.5 split made three mistakes possible that nothing else would catch:

| rule | enforced by | catches |
|---|---|---|
| Plumbing never touches Vue | `TestPlumbingDoesNotImportVue` | a `ref` in `session.js` — after which that file can only run inside a mounted app |
| A composable never imports another composable | `TestComposablesDoNotImportEachOther` | a flat set of files quietly becoming a graph |
| A component holds no branches | `TestComponentsHoldNoLogic` | the exact drift that grew `ba.js` to 179 lines holding the gate, the importer and the ticket transitions |
| Everything a template binds exists behind it | `TestTemplatesBindNothingUndefined` | a missing key in a `setup()` return — `undefined` at render, no error, no warning, just a blank where a badge should be |

Rule 14 is marked *prose only* on purpose: "the product needs no Node and no build step"
has no test, and pretending otherwise would be worse than admitting it.

All four new ones were **mutation-tested**: a copy of the tree with one violation injected
each — `const { ref } = Vue` in `session.js`, `corpus.js` importing `scope.js`, an `if` in
`ba.js`, and `{{ corpusTotals.docs }}` in the template. Each fails, in the right test, with
a message that says what to do.

The template check is deliberately coarse — does the identifier appear anywhere in the
module graph behind that template? — because a coarse check that runs beats an exact one
that needs a JS parser. Getting it there took three passes: string literals inside
expressions are not bindings (`:class="ok ? 'tip' : 'memo'"`), Vue's own `$emit`/`$refs`
are not either, and only the *head* of a member path is the app's to define
(`$event.dataTransfer.files` asks for `$event`, nothing more). That last one is positional,
so it is checked by position rather than by substring.

## About the antfu set

Asked for, and measured before adopting. `@antfu/eslint-config` on this tree:

- **255 packages, 113 MB** installed
- **3 real findings** — two unused regex capturing groups and a `parseFloat` — all three
  fixed in this commit
- **0 findings from its best rules.** eslint-plugin-vue's reactivity-loss checks, the ones
  that would have caught passing `props.documents` instead of a getter, only fire on SFCs
  and `<script setup>`. This repo has neither.

So it is wired up, and deliberately not in the gate: `make lint-js` installs it into
`.cache/` — the same throwaway tool cache `make diagram` already uses for mermaid — and
runs it. The config is tracked (`eslint.config.mjs`) with its stylistic layer off, because
this repo uses semicolons and double quotes and a reformat of every file is a diff nobody
can review for correctness. Nothing is committed for it: no `package.json`, no lockfile, no
`node_modules`. It currently reports zero.

The trigger for promoting it into `make check` is written at the top of the config and in
`web/frontend_test.go`: **the day TypeScript or `.vue` files land here.** At that point its
Vue rules start earning the weight, and the four Go tests above stop being the whole net.

## What was fixed on the way

- `diagram.js`: two capturing groups that nothing read are now non-capturing, and
  `parseFloat` → `Number.parseFloat` (the global coerces its argument; the namespaced one
  is the modern form).
- `web/frontend_test.go` itself tripped `gocritic` and `revive` on the first run — a
  one-argument `filepath.Join` and an empty `if` branch left where a comment was doing a
  condition's job. Fixed, so `make lint` is back at zero.
