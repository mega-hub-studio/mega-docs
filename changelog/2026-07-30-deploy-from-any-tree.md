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

## Landmine

**`pkill -f 'bin/knowledge'` matches `/opt/knowledge/bin/knowledge`.** Stopping a local
verification server that way takes the deployed service down with it; systemd brought it back in
under a second (`NRestarts` → 11) and health was green, so nothing on screen would have told
you. Kill a scratch instance by its port or its full path.

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

Not yet exercised: a real end-to-end where origin is genuinely ahead of the host. Nothing is
unpushed right now, so a live run would restart the service onto the commit it already serves.
