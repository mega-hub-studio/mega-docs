# 2026-07-28 — `make deploy`, and the version the UI shows

Two changes with one cause: a deploy on the host went wrong in a way nothing on screen could
have revealed, and "which version is running?" had no answer short of `journalctl`.

## What broke, exactly

`cd /opt/knowledge && git pull origin main` on the host stopped mid-way:

```
CONFLICT (modify/delete): web/ui/pnpm-lock.yaml deleted in 5e26898
  (one lockfile: npm, matching CI) and modified in HEAD.
error: could not apply 5e26898... one lockfile: npm, matching CI
```

`pull.rebase=true` is set on that host, and the host carried **one local commit** that had
never been pushed (`5e26898`, deleting `pnpm-lock.yaml`). Upstream had meanwhile modified that
file, so the upgrade turned into a half-finished rebase — on the machine serving the team,
with the old binary still running and no indication in the UI that anything was wrong.

A deploy checkout is a **mirror, not a branch**. That is the whole fix.

## `make deploy`

One target, and everything it adds over typing the four lines is a failure that has already
happened here:

| step | why it is not the hand-typed version |
|---|---|
| `git pull --ff-only` | refuses instead of rebasing. A local commit on the host now fails *before* the build, loudly, with the running binary untouched |
| `go test -run TestBuiltUIMatchesItsSources ./web/` | the host has no Node and cannot rebuild `web/dist`. A push that forgot `make ui` otherwise deploys a UI that predates the change, which reads as "the deploy did nothing" |
| revision printed before and after | so the output answers "did that change anything?" |
| health check, 20 tries | a failed restart leaves systemd retrying quietly and a stale page in somebody's browser |

Deliberately **not** in it: `make ui` and `make check-full` (Node and a browser, neither of
which a deploy host has) and any `git push`. It only moves this machine to what origin already
has. `UNIT` and `PORT` override the two host facts, and `PORT` is read from `.env` rather than
repeated in the Makefile — a port written twice is a health check that one day passes against
nothing.

## The version, and why not the obvious sources

The startup line already printed `build 442d57be`, and reusing it was the first idea. It is
**wrong for this question**: `build` is a hash of `web/ui` sources only (`web/dist/build.json`,
written by `stamp.js`), so a Go-only deploy reuses the identical string and looks like nothing
shipped. Rejected for the same reason: a `VERSION` file or `-ldflags -X` — the deploy is
`git pull && make build`, and a version somebody has to remember to bump is a version that
lies.

What shipped: **Go's own VCS stamp**, read with `debug.ReadBuildInfo()` in `cmd/server`. Every
binary built from a checkout carries `vcs.revision` and `vcs.modified` with no build flags at
all, so there is nothing to forget. `revision()` returns the short commit plus `+` when the
tree was dirty; empty for `go install` from a module proxy, which carries no stamp — and then
nothing renders, rather than a placeholder that would read as a fact.

It reaches the reader in three places, one fact each:

- `/api/health` → `"version":"e0a5159+"`, so `curl` answers it (the runbook's own check).
- the status line's end group → `@e0a5159+`, no icon (the `nes-icon name="branch"` first
  written there does not exist in 8bit-nes 0.8.0, and a missing icon renders an empty box with
  no warning), with the dirty-tree meaning in its `title`.
- the startup line, for `journalctl`.

`Runtime.Version` carries it — the one field in that struct that says nothing about an answer,
which its comment now says out loud.

## Verified

`make check` green (0 lint findings, every package ok). Against the built binary: startup line
`mega-docs e0a5159+ on http://127.0.0.1:8124`, health reporting `"version":"e0a5159+"`, and the
status line rendering `@e0a5159+` exactly once with the title text — matching
`git rev-parse --short=7 HEAD` (`e0a5159`) and a dirty tree. Console clean.
