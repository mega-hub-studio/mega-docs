# 2026-07-28 — `make check-full` on macOS: one red, one check that never ran, two shouts

First run of the final gate on this machine (macOS 26.5, arm64, Go 1.26.5, node 22, bash 3.2)
stopped at stage 4. Fixing it turned up three more things, all of the same family: a number
with two homes, a check that skipped, and noise that reads like failure.

Green after: `check-full: PASS`, with stage 2's `deadcode` actually running and stages 4–5
printing nothing but their verdict.

## The red: a CSS floor set *below* the gate that measures it

```
dev.html@1440 en: text boxes under 18 characters wide:
  [{"txt":"never, so probes need no","ch":16},{"txt":"X-BA-Pass — 401 wrong, 4","ch":16}]
```

Both cells are the last column of the routes table on the Dev page, and both sat at exactly
`min-inline-size: 14ch` — the floor `docsbase.html` sets for that column, added earlier for
this exact failure.

The floor did not hold because it was never big enough:

| | unit | in em | 
|---|---|---|
| `scripts/check-docs-ui.mjs` fails under **18 characters** | `width / (fontSize × 0.55)` | 9.90em |
| `docsbase.html` floors a cell at **14ch** | CSS `ch` — the `0` advance, 0.621em here | 8.69em |

One rule, two units, and the CSS one is 12% short. Nothing said so for as long as every
column happened to be wider than both numbers. Then [`1524e7f`](../web/dev.html) grew the
middle cell of that row — `admin`, `version`, and a sentence about the VCS stamp — the third
column settled onto its floor, and the gate reported the number the CSS had promised to
prevent.

Fixed at the floor, not at the prose: shortening that cell would have moved the trigger, not
the disagreement. **17ch** — 16ch converts to 18.07 of the gate's characters, which is a
rounding error away from red — and the conversion is written next to it, because the two
numbers are one rule and the next person to move either needs to see the other.

## `deadcode` had never run here

`make check` printed `skipped deadcode (go install …)` and carried on green. It is the
second time this is in a changelog: [`2026-07-28-audit-and-cleanup.md`](2026-07-28-audit-and-cleanup.md)
records the same discovery mid-session, installed the tool by hand, and concluded "the skip
message is doing its job; I was not reading it".

It is not doing its job. A skip that a human has to notice is the same failure mode as the
hardcoded Playwright path, and rule 17 loses its whole-program enforcer on every machine
where nobody typed the `go install`.

So `dead` installs it, the way `lint-deps` and `ui-deps` already install what they run — and
the case here is stronger than for either: the install needs only the Go toolchain, which a
box running `make check` has by definition. The old skip protected nobody.

The CI step that installed it is **deleted** in the same change. One installer per tool now,
which is what [`2026-07-28-lint-pinning.md`](2026-07-28-lint-pinning.md) did for the linter:
`deadcode` had drifted the other way from `golangci-lint` — present in CI, absent on a laptop.

Still **`@latest`**, still deliberately: that entry's reasoning is unchanged — it either finds
code unreachable from a main or it does not, so there is no rule set to move under us.

`GOBIN_DIR` (`go env GOBIN`, falling back to `GOPATH/bin`) is now where both targets look.
`GOBIN` matters: with it set and unread, `dead-deps` would install on every run and find
nothing.

## Two shouts on a green run

Neither is a macOS bug, but both surface here and one only here.

**`Terminated: 15`.** Both browser wrappers ended with

```
./scripts/check-docs-ui.sh: line 36: 12018 Terminated: 15   python3 -m http.server …
```

*after* `DOCS: PASS`. The cleanup trap kills the static server and leaves it unreaped, so bash
notices and reports it — but only when something follows the kill (stopping the browser
instance does) and, measured both ways, only under **bash 3.2**, which is what macOS ships.
`{ kill "$srv"; wait "$srv"; } 2>/dev/null` in both traps; the notice is gone.

**Twenty `HINT: no session set — this tab is shared`.** PinchTab 0.13.2 writes it on every
command, and `scripts/pinchtab.mjs` deliberately uses no session (see its `open()`: the
instance is already ours alone, and a session scopes "current tab" a second way).

The driver now runs `spawnSync` instead of `execFileSync`, drops that one line by name, and
prints the rest. Swallowing stderr wholesale was the first attempt and was wrong: **PinchTab
reports `Error 500: …` on stderr and exits 0**, so an inherited stderr was the only place a
real failure ever appeared. The same change fixes the opposite bug in the catch — it had
always read `e.stderr`, which is null unless stderr is piped, so a genuine non-zero exit
reported `Command failed` and nothing about why.

One line still prints on a cold start: `Error 500: new tab: create target: context canceled`,
which `open()` retries and the next attempt fixes. Left visible. It is true, and the
alternative is whitelisting an error string.

## Looked like a fourth hole, and is not

`make check` prints this on any machine whose terminal is a VS Code one — which is most of
them here:

```
[@antfu/eslint-config] Detected running in editor, some rules are disabled.
```

`VSCODE_PID` is set in the integrated terminal, `isInEditorEnv()` reads it, and three rules
drop from `error` to `warn`: `prefer-const`, `unused-imports/no-unused-imports`,
`test/no-only-tests`. That reads exactly like the `deadcode` skip above, one paragraph after
writing it up.

It is not, and the reason is one flag: `web/ui/package.json` runs `eslint . --max-warnings 0`,
so a warning fails the run as hard as an error and the verdict is identical either way. What
editor mode actually changes here is `disableRulesFix` — those three stop being auto-fixed by
`npm run lint:fix`, which is not the gate and is the behaviour antfu intends.

Left alone: forcing `isInEditor: false` would buy nothing today (rule 20's "what breaks
without it?"), and the answer is nothing. Recorded because the line is alarming, the check
takes ten minutes, and the next person to see it should not have to redo it.

## Second pass: what the quiet gate then showed

With the shouting stopped, `make check-full` printed **813 lines** on a green run and the
verdict was three of them. Everything below came out of reading the rest.

**813 → 139 lines**, and 115 of what is left is Vite's per-asset table — kept deliberately:
CLAUDE.md names chunk sizes as the only signal that would catch mermaid moving into first
paint.

| what | it was | it is |
|---|---|---|
| `rendocs` progress | `fmt.Fprintf(os.Stderr, …)`, so it survived the `>/dev/null` both wrappers already wrote — 12 lines a run | `fmt.Printf` — the redirect now does what it says, and a human running it by hand still sees it |
| the measurement JSON | ~650 lines printed on **green**, ahead of `DOCS: PASS` | printed only when red, where every number that produced the failure still is |
| `pinchtab doctor` guard | a skip that could not fire | deleted |
| "free the port first" | a no-op | an id looked up from `instances` |
| the browser | leaked, one per run | stopped, and the stop is checked |

### Three guards that had never once run

Each was written for a real failure, each silenced its own error, and each was doing nothing.

- **`pinchtab doctor`** — 0.13.2 has no `doctor` (the command is `health`), and it would not
  have mattered: **no pinchtab subcommand can be gated on its exit code**. Measured:
  `pinchtab bogus-subcommand` exits 0, and `health` against a refused connection exits 0 while
  printing the refusal. Deleted rather than ported to `health`, because the honest detector is
  the `instance start` twenty lines below it. A box with no pinchtab still skips.
- **`instance stop` with no id**, the line whose comment says it frees a port left by an
  interrupted run: the CLI answers `Error: accepts 1 arg(s), received 0`, into the
  `2>/dev/null` beside it. So the guard against `409 port already reserved` had never run, and
  the 409 is exactly what turned up the day the failure message started quoting pinchtab
  instead of guessing at the cause — which is the whole argument for quoting it.
- **the EXIT trap's `instance stop`**, which reported success and left the instance `running`.
  Cause found by printing what it actually said: `404 page not found`. `PINCHTAB_SERVER` was
  exported for the node command's convenience, and it retargets **every** pinchtab call —
  including a trap eighty lines away, at an instance's own server, which does not serve the
  instance API. Not exported any more; the wrapper puts it on the command line that needs it.
  That was the leak feeding the 409, so both ends of that loop are closed.

### One rig, not two copies of one

`scripts/guide-rig.sh` is new and is not a new layer: it is the ~50 lines both wrappers
already had, minus one copy. The trigger was counting — five fixes in this session
(the reaped `kill`, the `doctor` guard, the readiness wait, the stale sweep, the error
message) each had to be applied **twice**, which is rule 17's drift on a schedule. Each
wrapper is now four variables and the node call.

The one behaviour it adds, because it is where it belongs: `instance start` answers
`"status": "starting"` and returns, so the driver's boot navigation raced a browser that was
not up and lost about half the time — one `Error 500: navigate: context canceled` per run,
retried green. It waits for `instances` to say `running` instead. Two runs each of both
checks: no 500, nothing leaked.

### Also gone

`web/ui/package.json` had `"check:full": "make -C ../.. check-full"`, referenced nowhere and
documented nowhere — a second door to the one command CLAUDE.md calls the gate. It was also
the only finding `npx knip` had (as "unlisted binary: make"); knip is clean now.

## Verified

`make check-full` end to end on this machine: bundle rebuilt, Go tests + vet + golangci-lint
at zero + **deadcode 0 unreachable** + secret scan, both binaries, `DOCS: PASS`,
`WALKTHROUGHS: PASS`. `check-ui` and `check-wt` re-run twice more after the driver change,
same verdicts, no stray lines.

After the second pass, and each of these measured rather than assumed:

- `make check-full` green, **139 lines**, `pinchtab instances` unchanged before and after.
- `check-ui` and `check-wt` twice each on the shared rig: `PASS`, no `Error 500`, no leak.
- The **red** path, forced by tightening one assertion to something impossible: 474 lines —
  the full measurement JSON, then the failures, then `DOCS: FAIL`, exit 1, trap clean.
- `npx knip` in `web/ui`: nothing.
- `make dead`: ran, 0 unreachable.

Not verified here: CI. The deleted install step means the first push is what proves
`make dead` installs the tool on a runner as well as on a laptop — and the `darwin` job added
alongside it is the other thing only a push can answer.
