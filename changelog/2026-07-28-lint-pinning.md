# 2026-07-28 — The lint gate against upstream's own recommendations: pinned, and a `disable:` block that disabled nothing

Read <https://golangci-lint.run/docs/> properly and compared it with what this repo does.
The config was already explaining itself — every deviation had its reason next to it — but
four things it did disagreed with the official guidance, and two of them were dead weight
that had been *costing* something rather than merely sitting there.

Measured before touching anything: golangci-lint **2.12.2**, which is the current release,
**0 issues**. Every change below was applied against that baseline and re-verified at zero.

## 1. The version is pinned, and the thing that lints installs it

Both halves of how the linter got onto a machine were what upstream tells you not to do:

> "Using `go install`/`go get`, 'tools pattern', and `tool` command/directives
> installations **aren't guaranteed to work**. We recommend using binary installation."
> — [install/local](https://golangci-lint.run/docs/welcome/install/local/)

> "It's **highly recommended** installing a specific version of golangci-lint available on
> the releases page." — [install/ci](https://golangci-lint.run/docs/welcome/install/ci/)

CI ran `go install …@latest`, so it hit both at once. And the bill had already been paid —
`goconst` and a new `gosec` rule went red in CI while a laptop with an older binary said
zero. The response at the time was to make `make lint` **print** the version and ask a
human to compare it with `check.yml`; that is a symptom fix, and CLAUDE.md's trap list
carried it as a permanent hazard.

Now: `GOLANGCI_VERSION := v2.12.2` in the Makefile is the only place a version is written.
`make lint` depends on `lint-deps`, which checks the installed binary's version and
installs the pinned one via upstream's `install.sh` when it is absent *or different*. CI
installs **no linter at all** — it runs `make check`, which brings its own.

That is the `ui-deps` pattern, deliberately: the same repo already decided that a target
whose job is to build the front end should install node_modules rather than tell you to.
Nobody wants "go install the other version" from a target whose job is to lint.

The trap entry in CLAUDE.md is **deleted** rather than reworded. The mechanism removed the
hazard, and a trap list describing a hazard that cannot happen is the stale doc rule 24 is
about.

## 2. `default: none` — and the entire `disable:` block was doing nothing

`linters.default` defaults to `standard`, which is **five** linters: `errcheck`, `govet`,
`ineffassign`, `staticcheck`, `unused`. The effective set is `standard ∪ enable \ disable`.

Checked all 38 names in the `disable:` block against `standard ∪ enable`: **the
intersection is empty.** Not one of them was enabled, so not one of them was being
disabled. Deleting the whole block and re-running gave the same **0 issues** — 69 lines of
config that changed nothing.

Worse than nothing, though, because an unknown linter name is a **hard error** and applies
inside `disable:` too. So each of those 38 names was a tripwire on every upgrade for a
suppression that did not exist — and that tripwire had already distorted the config: a
six-line comment existed explaining why the name `modernize` could not be written down,
since it only exists from golangci-lint 2.9 and would lock out an older binary.

Pinning the version removes that constraint entirely, so:

- `default: none`, and the `enable:` list is now the **complete** set — the five standard
  names written out explicitly. No union with a default you have to look up, and no linter
  arriving on an upgrade to shout at a tree that was green.
- The `disable:` block is gone; the four decisions with a repo-specific reason (`wsl_v5`,
  `paralleltest`, `dupl`, `noctx`) and the four that are simply absent dependencies
  (`sloglint`, `loggercheck`, `testifylint`, `exptostd`) are **comments**. The ~30 that had
  no reason beyond "the recommended config already explains it" are not named at all —
  that was a second copy of somebody else's list.
- **`modernize` is enabled.** Verified at zero on this tree.
- `formatters.disable: [gofumpt]` went the same way: formatters are opt-in, so disabling
  one that was never enabled is the same no-op. The reason it stays out (it reads the
  dotless `knowledge-engine` module path as stdlib and merges every internal import into
  the std group) is a comment now.

## 3. A lint warning fails the gate

`exclusions.warn-unused` defaults to `false`. Turned on, it names any exclusion rule in
`.golangci.yml` that stopped matching — and this config has rules pointing at specific
paths (`cmd/ingest/`, `internal/config/`) that will rot silently on a rename.

Verified it actually fires by planting a rule at a path that does not exist:

```
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules:
  [Path: \"internal/nonexistent/\", Linters: \"gosec\"]"
```

golangci-lint exits **zero** on that, which would make it scroll past on every green run
— which is how it stays forever. So `make lint` greps its own output and fails on
`level=warning`. An exclusion nobody needs is exactly the dead config rule 24 calls a lie
in the gate. Zero stale rules today, so this costs nothing now and is pure ratchet.

## 4. `gofmt -l .` deleted from `make check`

Two problems, one line.

**Redundant**: `formatters.enable: [gofmt]` means `golangci-lint run` already reports it.
Verified by planting a badly-formatted function — `run` said
`File is not properly formatted (gofmt)`. So `make check` was checking formatting twice.

**Mis-scoped**, and this is the real one: `gofmt -l .` walks the whole tree, including
`web/ui/node_modules`, where one npm dependency (flatted) ships **two** Go files. That is
precisely the trap `PKGS := ./cmd/... ./internal/... ./web` exists to close, reopened by a
shorter command. It had never fired only because that file happens to be gofmt-clean; it
would have fired the day a dependency shipped unformatted Go, blaming this repo for it.

`lint` covers gofmt and covers it over `$(PKGS)`, so the line is gone rather than fixed.

## Considered and rejected

- **`exclusions.presets`** (`std-error-handling`, `common-false-positives`, `comments`,
  `legacy`). These are broad, *implicit* exclusions. The hand-rolled
  `errcheck.exclude-functions` here names exactly five functions with the reason attached —
  adopting a preset would trade explicit for implicit in the one file whose whole purpose
  is that every suppression can be explained. Not adopted.
- **`golangci-lint-action`** in CI, which the docs do recommend for its caching and its
  GitHub annotations. It would mean a **second** path that runs the linter, parallel to
  `make check` — the one gate nobody can skip. Measured the thing the cache would buy:
  cold **3.26s**, warm 0.53s. Paying a second lint path and a cache step for 2.7 seconds
  fails rule 20's "what breaks today without it?". If PR annotations are wanted later, add
  a **non-gating** job; do not move lint out of `make check`.
- **Pinning `deadcode`** too. It has no release binaries, and unlike a linter suite it
  either finds code unreachable from a main or it does not — there is no rule set to shift
  under us. Left on `@latest`, knowingly.

## The gap this opened, and where it went

`go.mod` said **`go 1.22.5`** — July 2024, past end of support — and `run.go` derives its
lint target from it, so it **capped** what `usestdlibvars`, `intrange` and `modernize` were
allowed to suggest. That was named here as a gap rather than fixed, on the grounds that it
changes what the *deployed* binary compiles against and deserves its own verification.

It was then done, in the same session but as its own change:
[`2026-07-28-go-1.26.md`](2026-07-28-go-1.26.md). It has a **deploy prerequisite** — read
that entry before the next deploy.

## Verified

`make check` clean end to end, and both new failure paths driven rather than assumed:

- a planted stale exclusion rule → `make lint` exits non-zero with the message naming it
- `GOLANGCI_VERSION` overridden to a version that does not exist → `lint-deps` detects the
  mismatch and fails at the download instead of silently using the binary already there

Net on `.golangci.yml`: **200 → 182 lines**. The `disable:` block that went was 69 of
them; the reasons worth keeping came back as comments, which is the trade this was for —
fewer keys the linter parses, the same facts a reader gets. Plus one place a version is
written, two new things the gate catches, and one class of "green here, red in CI" closed
by construction.
