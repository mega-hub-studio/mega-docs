# 2026-07-30 — `make deploy` from any tree, because the wrong one reported success

Continues [`2026-07-28-deploy-and-version.md`](2026-07-28-deploy-and-version.md), which built
`make deploy` around four guards. It had a fifth failure it could not see: **which checkout it
was standing in.**

## The bug

The supervisor names its binary absolutely:

```
WorkingDirectory=/opt/knowledge
ExecStart=/opt/knowledge/bin/knowledge
```

`make` is relative to wherever it was typed. So `make deploy` in a dev tree did every step
successfully against the wrong directory: `git pull --ff-only` **here**, `make build` writing
`./bin/knowledge` **here**, `$(RESTART)` restarting the unit — which re-execs
`/opt/knowledge/bin/knowledge`, untouched — then a health check that passes because the *old*
binary is serving fine, and `deployed: <sha>` printed from the dev tree's HEAD.

Every line of output said it worked. Nothing had moved. That is the exact class of failure the
07-28 entry created this target to prevent, reopened from a direction it did not look.

The guide made the same mistake honestly: it taught `cd /opt/knowledge && make deploy`, so the
`cd` *was* the guard — an instruction, enforced by nothing.

## The fix

`DEPLOY_DIR ?= /opt/knowledge`, and `deploy` became a dispatcher over `deploy-here`:

- Same directory → run `deploy-here`, whose four guards are unchanged.
- Different directory → print which tree is being deployed, then `$(MAKE) -C "$$there" deploy`.
- Not a directory, or not a checkout → refuse, naming `DEPLOY_DIR`.

**It hands over to `deploy`, not to `deploy-here`.** A deploy checkout still carrying an older
Makefile has no `deploy-here`, and its own `deploy` is correct *in place* — so the first run
after this change installs the dispatcher rather than failing on it. Verified: the delegated run
reported `Makefile:309`, the old target's line.

Two targets rather than one, because **`exec` cannot end a make recipe**: every recipe line is
its own shell, so a line that `exec`s is replaced and make cheerfully runs the next line anyway.
The dispatch has to be a target boundary.

## Decisions

**Still no `git push`, still a mirror not a branch.** The 07-28 decision stands; this only
changes *where* the target operates, never what it operates on. What it adds is the note — run
from a tree holding commits origin does not have, it says
`note: N commit(s) here are not pushed` before starting, because "identical before and after"
was previously something the reader had to interpret. A note, not a refusal: redeploying or
rolling back to what origin has is a legitimate reason to run it.

**Cross-OS needed nothing.** `RESTART`/`STATUS` already branch on `uname -s` (launchctl vs
systemctl), and the delegated `make -C` re-evaluates them in the target tree, so macOS + launchd
works unchanged. Only `DEPLOY_DIR`'s default is Linux-shaped, and it is overridable — the plist
is what names the checkout there.

**Rejected: build here, install the binary there** (`install bin/knowledge /opt/knowledge/bin/`).
Fastest possible loop, and it breaks the invariant that earns this target its trust: the deploy
checkout mirrors origin, and `vcs.revision` comes from *that* checkout. Installing a dev binary
makes `/api/health` report a revision no reviewer can check out, `+`-suffixed on a dirty tree.
The round trip through origin is the provenance.

**Rejected: deriving `DEPLOY_DIR` from the supervisor** (`systemctl show -p WorkingDirectory`,
and the launchd equivalent). It would keep the path from being written twice, which is this
Makefile's own rule — but it is two more shell-outs, two more OS branches, and it fails opaquely
when the unit is not installed yet. A default plus an override is the cheaper correct thing.

## The fifth guard, found the hard way an hour later

`make deploy UNIT=knowledgey` — a typo — got **all the way through pull and build** and died on
`Failed to restart knowledgey.service: Unit knowledgey.service not found`. The target failed
loudly and non-zero, which is correct, and the state it left was still wrong:

```
running process started 11:09:23   →  /proc/<pid>/exe: (deleted)
binary on disk           11:40:40  →  31 minutes newer
/api/health                        →  {"ok":true, "version":"9dc043a"}   ← the OLD commit
```

The new binary had replaced the old one on disk while the running process kept serving the
deleted inode, reporting `ok:true` the whole time. The health check never ran, because the
restart it verifies is what failed. Half-deployed, and green.

So `deploy-here` now asks the supervisor **first**, before the pull and before the build:
`KNOWN` is `systemctl cat $(UNIT)` on Linux and `launchctl print gui/$UID/$(LABEL)` on macOS —
the question "have you ever heard of this job", which both answer by exit status. A typo now
costs 0.0s and changes nothing. That is the same argument `--ff-only` makes: refuse while the
old binary is still running, rather than discover it mid-deploy.

`NAMED` exists only so the message names the variable the reader must fix (`UNIT=` vs `LABEL=`),
since which one is wrong depends on the OS.

## Landmines

**`pkill -f 'bin/knowledge'` matches `/opt/knowledge/bin/knowledge`.** Stopping a local
verification server that way takes the deployed service down with it; systemd brought it back in
under a second (`NRestarts` → 11) and health was green, so nothing on screen would have told
you. Kill a scratch instance by its port or its full path.

**A leaked rig server makes `make check-ui` measure a *stale* render and fail on pages you never
touched.** `scripts/guide-rig.sh` serves `mktemp -d` on port 8123 and kills it from an EXIT trap;
a run interrupted before the trap leaves it bound. The next run's own `python3 -m http.server`
then fails to bind — silently, it is `>/dev/null 2>&1` — and the readiness probe that follows
succeeds *against the leaked server*, so the browser measures last run's HTML. It reported
`table rows stacked on a laptop` on `dev.html` and `deploy.html`, and `language toggle stuck at
en`, none of which were true of the tree. Worse, each failed run leaks another one, so it
compounds.

Proving it was not the edit under review took reverting `web/deploy.html` to `HEAD` and watching
the baseline fail identically. The cheap check first, next time:

```
pgrep -af 'http.server 8123'      # must be empty before check-ui
```

The rig already sweeps stale *pinchtab instances* by port for the same reason (`409 port already
reserved`); it does not yet sweep the HTTP server, and a foreign server answering the readiness
probe is the more dangerous of the two because it looks like a pass. Not fixed here — it is
outside this change — but a `check-ui` failure naming a page you did not edit should send you to
that `pgrep` before the diff.

**The dispatcher runs the *target* tree's Makefile, so a new guard is not in force until it is
committed and deployed.** Re-running `make deploy UNIT=knowledgey` from the dev tree to test the
new refusal delegated into `/opt/knowledge`, which still carried the old Makefile, and did a
second full pull-build-restart cycle instead. Harmless here — same commit in and out, so the
rebuilt binary was identical and health stayed on `d4d82ae` — but a guard has to be tested
against `deploy-here` directly (`make deploy-here UNIT=…`) until it has shipped.

## State outside git

Nothing new. `/opt/knowledge` is at `a412507`, clean, `main` tracking `origin/main`, with
`pull.rebase=true` still set — which `--ff-only` overrides per invocation, and which the WSL
runbook's step 2 tells you to fix with `git config pull.ff only`.

## Verified

Guard paths, in isolated clones, with no build and no restart of the real service:

| run | output |
|---|---|
| `DEPLOY_DIR=/no/such/dir` | `refusing: … is not a directory` |
| `DEPLOY_DIR=/tmp` | `refusing: /tmp is not a git checkout` |
| `DEPLOY_DIR=.` (dirty tree) | dispatches to `deploy-here` → `refusing: working tree is dirty` |
| clone A (2 unpushed) → clone B | `note: 2 commit(s) here are not pushed`, `deploying …/B`, then B's own dirty refusal, `make[1] Error 1` → `make Error 2` |
| `deploy-here UNIT=knowledgey` | `refusing: this machine has no UNIT=knowledgey — nothing would be restarted`, 0.0s, before pull |
| `deploy UNIT=knowledgey` once the guard had shipped to `/opt/knowledge` (`65a2a4e`) | `deploying /opt/knowledge` → the same refusal → `make[2] Error 1` → `make[1] Error 2` → `make Error 2`, and afterwards `/api/health` still `65a2a4e` with `/proc/<pid>/exe` on the current binary. The original bug, re-run, now changes nothing |

And live, from the dev tree, repairing the half-deploy above:

```
make deploy
  deploying /opt/knowledge
  before: d4d82ae
  … pull (already up to date) → stale check ok → build → sudo systemctl restart knowledge
  deployed: d4d82ae — {"ok":true,…,"version":"d4d82ae"}
```

`/api/health` went `9dc043a` → `d4d82ae`, the process is a fresh pid on the current on-disk
binary (no longer a deleted inode), and the served bundle contains `ASK THIS`, `RECOMMENDED` and
`callout clarify` — so the clarify work is what is running, not just what is committed.
