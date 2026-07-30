# 2026-07-30 — `/ship`: one fixer, the gate, then a ponytail read

The ask was "auto-format after implementing, chained into the gate, plus a ponytail-lens
review". Most of the first half turned out to exist already, so what shipped is smaller than
the request and the difference is recorded here rather than re-derived.

## What was already there

Formatting is **verified** by the gate, both languages, and has been:

| | verified by | in |
|---|---|---|
| Go | `gofmt` under `formatters.enable` in `.golangci.yml` — reported by `golangci-lint run` like any finding | `make lint` → `check` → CI |
| JS · Vue | antfu `stylistic` (indent 2 · single quotes · no semi) via `eslint . --max-warnings 0` | `make lint-js` → `check` → CI |
| everything | `.editorconfig` (tabs for Go, spaces elsewhere, no trimming in `.md`/`.html`) | editors, not a check |

Adding a format *step* to `check` would have repeated a mistake this repo already deleted:
`changelog/2026-07-28-lint-pinning.md` removed `gofmt -l .` from `make check` as redundant
(golangci-lint already reports it) **and** mis-scoped (it walks `web/ui/node_modules`, where an
npm dependency ships Go files).

## What was actually missing

`make lint-fix` was **Go only** — `golangci-lint run --fix $(PKGS)`. `web/ui/package.json` had
`lint:fix` and no Make target called it, so "the formatter is in the gate" was true while "one
command formats this repo" was false. `lint-fix` runs both halves now, the JS one **inline**
rather than as its own target — see the review note below for why that changed mid-change.

## Decisions — do not relitigate

**The gate does not mutate.** `lint-fix` is deliberately not a step of `check`. A gate that
rewrites files reports green on code nobody reviewed, or hides a real violation as
"auto-fixed" — and CI has no diff guard over source (only over `web/dist`). Fixing is a
developer action; verifying is the gate's.

**The agent review is not in `check-full`.** CI has no LLM, and CLAUDE.md already records
*"`ultracode` … Opt-in only, never the default for a change. `make check-full` is the audit of
record"*. So the review lives in `/ship`, **after** the gate is green — an agent never reads
code the machine has already rejected, which is also the cheaper order.

**One name, not two.** No `make fmt`. `lint-fix` already existed and is already documented, and
`fmt` would understate it — it applies every autofixable lint rule, not only formatting.

**Still no `gofumpt`, still no `eslint-plugin-format`.** Both rejected with reasons on record
(module-path heuristic; Go templates with `<% %>` that no HTML formatter parses). A fixer that
runs more formatters is not the same command.

**Report-only review.** Applying findings re-dirties the tree and voids the green gate that just
ran, so `/ship` says what it would change and asks.

## The bug this turned up, which was older than the change

`lint-js`'s skip guard had **never once worked**:

```makefile
lint-js:
	@[ -d web/ui/node_modules ] || { echo "  skipped lint-js (…)"; exit 0; }
	@cd web/ui && CI=1 npm run --silent lint
```

Make runs every recipe line in its own shell, so `exit 0` ends *that* shell and make proceeds to
the next line anyway. Measured by moving `node_modules` aside:

```
  skipped lint-js (run `make ui` to install web/ui)
sh: 1: eslint: not found
make: *** [lint-js] Error 127
```

It printed the skip **and then failed**. CLAUDE.md documented a behaviour that did not happen —
the same shape as the `exec`-cannot-end-a-recipe trap in
[`2026-07-30-deploy-from-any-tree.md`](2026-07-30-deploy-from-any-tree.md). Both guards are one
`if … else … fi` line now, and both were re-measured in both directions.

It was latent because nothing on a developer box or in CI ever hits the no-Node path. Copying
the pattern into `lint-fix`'s new JS half is what exposed it.

## The chain caught something in its own construction

Step 3 was run on this change, and ponytail at `full` flagged two things worth keeping as a
record that the review layer is not decorative:

- **`lint-js-fix` was a target with exactly one caller**, and its own comment said "nothing else
  should call it" — which is an admission that the name earns nothing (rule 20's three
  questions: one caller, no knob, nothing breaks without it). Inlined into `lint-fix`; one
  target and one `.PHONY` entry deleted. `npm run lint:fix` is the one-liner for anyone who
  wants only that half.
- **The "never part of `check`" rationale was written twice**, in the Makefile comment and again
  in CLAUDE.md's *Commands* block (rule 17). CLAUDE.md keeps the *what* and points at the
  Makefile for the *why*.

Both were in the approved plan, and both were smaller after the review than before it. That is
the argument for the step existing at all — the gate was green through both.

## Also measured, and it shapes step 1

`eslint --fix` exits **non-zero** when a finding remains that it cannot repair — an unused
variable, for instance. So `lint-fix` fails on a tree it has partly fixed, and the failure
propagates (`make[1] Error 1` → `make Error 2`). `/ship` therefore treats step 1's exit code as
information and lets `check-full` be the verdict; without that, every unfixable lint finding
would abort the chain before the gate could name it properly.

## Verified

| run | result |
|---|---|
| append `const _probe = "x";` to `web/ui/src/lib/session.js`, `make lint-fix` | rewritten to `const _probe = 'x'` — quotes and semicolon both fixed, so the JS half really runs |
| `gofmt -l ./internal ./cmd ./web` after | empty |
| `mv web/ui/node_modules` aside, `make lint-js` and `make lint-fix` | `skipped …`, exit **0**, no eslint invoked — the guard works now |
| the same two with `node_modules` present | exit 0, both do their work |
| `make check-full` | PASS |

Not verified in this session: that `/ship` is *discoverable* as a slash command. Claude Code
reads `.claude/commands/*.md`, and this repo had no such directory before, so the listing was
built before the file existed. It should appear in the next session; if it does not, that is a
harness question, not a content one.

## State outside git

None. `.claude/commands/` is new; `TestVendoredSkillsMatchTheirRouting` globs only
`../.claude/skills` (`web/embed_test.go:668`), so the directory does not touch the gate.
