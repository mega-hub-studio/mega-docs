---
description: Fix both languages, run the gate, then review the diff through ponytail at full.
---

Four steps, in order. **Stop at the first one that fails**, report why, and do not continue —
in particular, never run step 3 over code the gate has rejected.

## 0 · Precondition: the rig's port is free

```bash
pgrep -af 'http.server 8123' | grep -v zsh
```

Must print nothing. `scripts/guide-rig.sh` serves a `mktemp -d` on 8123 and kills it from an
EXIT trap; a run interrupted before that trap leaves it bound. The next run's own server then
fails to bind — silently, it is `>/dev/null 2>&1` — and the readiness probe that follows
succeeds *against the leaked server*, so `check-ui` measures the previous render and fails on
pages nobody touched. Each failure leaks another, so it compounds.

If it prints a pid: kill that pid, remove its `-d /tmp/tmp.*` directory, then continue.

## 1 · Fix what a fixer can fix

```bash
make lint-fix
git diff --stat
```

`lint-fix` runs `golangci-lint run --fix` over `$(PKGS)` and ESLint's `--fix` over `web/ui`.

Then show the `git diff --stat`. A formatter's edits are changes nobody wrote, so they have to
be visible before the gate hides them behind a green line.

**Ignore its exit code.** ESLint exits non-zero whenever a finding remains that `--fix` cannot
repair (an unused variable, say) — measured. That is information, not a verdict; step 2 is the
authority. Report what it could not fix and move on.

Order matters and is already right: this must run **before** `make ui`, and `check-full` starts
with `ui`. Formatting `web/ui/src` after the bundle is built makes
`TestBuiltUIMatchesItsSources` red.

## 2 · The gate

```bash
make check-full
```

Red → stop here. Name the stage that failed and quote its output. No review.

Green → read the output for `skipped` lines before calling it covered. `check-ui` and `check-wt`
skip silently when PinchTab is not on PATH, and a skipped check reads exactly like a passing one.

## 3 · Review the diff, ponytail at `full`

Only when step 2 is green. Load the `ponytail` skill at `full`, then read everything the change
touched — `git diff` **plus the contents of every untracked file**, because a new file is not in
`git diff` at all:

```bash
git status --porcelain
git diff
```

Judge it through this repo's own rules, in CLAUDE.md's precedence order — a rule outranks the
skill, so where ponytail and a rule disagree, the rule wins and the disagreement goes in
`changelog/`:

| lens | what it catches |
|---|---|
| rule 17 · 20 | an abstraction with one caller · a knob nobody turns · a second copy of a fact |
| rule 21 | a new test file, a fixture, a mock, any scaffold added for this change |
| rule 22 | complexity not behind one seam · a call site that does not read as intent · a name `grep` cannot resolve in one hop |
| rule 24 | a deferred marker · a `//nolint` that does not name its linter *and* reason · a doc left behind |
| four-layer table | a branch in `<script setup>` · a composable reaching for another's state · `src/lib/` importing Vue |

Output in ponytail's own shape: the finding first, then at most three short lines. No essays —
a paragraph defending a simplification is complexity smuggled back as prose.

Then add one line ponytail does not ask for, because a review that only lists problems never
says how far it looked: **what you read and deliberately did not flag.**

**Report only — do not apply.** Applying re-dirties the tree and invalidates the green gate that
just ran. If there are findings, say what you would change and ask; on a yes, apply them and
return to step 1.
