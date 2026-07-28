# 2026-07-28 — Corpus git-sync deleted: the brief's "Removed" line, taken literally

`scripts/corpus-sync.sh` is gone, and with it two of the brief's `Removed` items in one
deletion: **Git sync** (the script committed and pushed the corpus) and **Folder watch**
(the `corpus-sync.path` unit it was driven by).

## The risk, raised and then accepted

This was flagged before it was done, because the script was the **only backup that existed**
on a live deployment and its intended replacement — an off-box DB backup — has not been
built. `2026-07-28-sot-decision.md` had put that backup *before* the source-of-truth
inversion for exactly this reason.

The decision was to delete anyway, and the reasoning is worth keeping: carrying automation
the brief removes, in order to protect a backup the brief never asks for, is the redundancy
this cleanup exists to eliminate. `CORPUS_DIR` is still the source of truth and
`knowledge.db` is still derived from it, so nothing about recoverability *from the corpus*
changed. What changed is that keeping a copy of the corpus is now a human's job.

**State it plainly, because it is easy to rediscover as an outage:** nothing backs up the
corpus. `git -C docs commit` is the whole story, run by whoever operates the instance. The
Deploy page says so in both languages now ("by hand — nothing automates it"), rather than
implying automation that no longer ships.

## Cleaning up the host

The repo no longer carries the script, but this box still has the units enabled — they will
fail on their next trigger, looking for a file that is not there. They need root, so they
are not done from here:

```bash
sudo systemctl disable --now corpus-sync.path corpus-sync.timer
sudo rm -f /etc/systemd/system/corpus-sync.{path,timer,service}
sudo systemctl daemon-reload
systemctl is-active knowledge          # unaffected — a different unit
```

`knowledge.service` is untouched: it never depended on the sync.

## What was removed, exactly

| | |
|---|---|
| deleted | `scripts/corpus-sync.sh` |
| deleted | the two "Commit what the app writes, automatically" rows in `web/deploy.html` (EN + VI) |
| amended | the Deploy page's *Back up* row — now says the git commit is manual |
| amended | `README.md` *Now vs vNext*: Git sync · Folder watch → decided |
| amended | `README-MEGA-DOCS.md` — both `Removed` lines marked done |
| left alone | `changelog/2026-07-27-wsl-deploy-and-import.md`, which documents the units as they were. A changelog entry records what was true on its date; it is not edited when the world moves on — this file is how the world moving on gets recorded |
