# 2026-07-28 — A second host, on macOS: the supervisor was the only thing in the way

A macOS instance is being stood up for a **different company's** knowledge, alongside the
WSL host. The repo work to make that possible is done and in this commit; the host itself
is not built yet — §3 is what a next session picks up.

The finding worth keeping above everything else: **the binary was already cross-OS, and
exactly one thing was not — the supervisor.** Build, `.env`, health check and the way the
port is published are identical on both. So this was three files, not a project.

---

## 1. What changed in the repo

### `make deploy` chooses its supervisor from `uname -s`

Two variables (`RESTART`, `STATUS`), not a second target. A macOS copy of `deploy` would
carry its own drifting version of the four guards the target exists for — the dirty-tree
refusal, `--ff-only`, the stale-`web/dist` test and the health check.

What it fixed is a **half deploy**: on a Mac the old target refused nothing, pulled, built
the new binary, then died on `systemctl`. The old process kept serving, and the log read
like a deploy had happened. `launchctl kickstart -k` was chosen over `bootstrap` on every
run for the same reason `systemctl restart` is: it fails on a job that was never installed
instead of quietly inventing one, so a missing agent cannot look like a successful deploy.

`UNIT` still names the systemd unit and `LABEL` the launchd job; both are overridable.
`PORT` is still read from `.env`, so a second instance on another port needs no Makefile
change — which is what makes the dev-alongside-prod layout in §3 free.

### The macOS plist on the Deploy page was not a plist

The card shipped four `<key>` lines with no `<?xml>`, no DOCTYPE and no `<plist><dict>`
wrapper, presented as the whole file. **`plutil -lint` says `OK` on it** — it reads the
fragment as an old-style plist — while `plutil -p` shows what it actually parsed: the bare
string `"Label"`. So the obvious way to check the file before loading it returns a green
light, and `launchctl` then fails with an I/O error that names nothing.

That is the trap worth remembering, not the missing tags: **a linter that accepts a file is
not a linter that understood it.** The page now carries a complete plist, so the trap
cannot fire from a copy-paste any more, which is why it is recorded here and not there.

Three more facts the card was missing, each an hour on its own:

- `launchctl load` is deprecated — `launchctl help` itself answers
  `load  Recommended alternatives: bootstrap | enable`. The verb is
  `launchctl bootstrap gui/$UID <path>`.
- **launchd does not expand `~`.** Every path in a plist is absolute or it is wrong.
- **There is no `journalctl`.** Without `StandardOutPath`/`StandardErrorPath` the output
  goes nowhere, and the failure looks like a service that started and did nothing.

Not a defect, checked before it was written down: a missing `RunAtLoad` is fine —
`KeepAlive` true starts the job by itself.

The deploy directory in the card moved from `/opt/knowledge` to a path under `$HOME`. The
card's own claim is that a user agent needs no root, and `/opt` contradicts it: creating it
needs `sudo`, after which `git pull` and `make build` do too.

### A Mac is not an always-on server, and it fails looking like an app bug

A new gotcha callout, because this is the macOS half of the WSL "systemd only runs while
the distro is up" note and presents identically to a reader: the URL stops answering and
nothing is wrong with the app. Measured on the host being built —
`pmset -g`: `sleep 1`, `womp 1` on mains and `0` on battery, `powernap 1`. Neither a
tunnel nor a tailnet reliably wakes a sleeping laptop.

`sudo pmset -c sleep 0` is the fix, and **mains-only is the point**: a server while plugged
in, a laptop on battery. `caffeinate -s` around the binary does the same job on battery
too, which is a flat tax on a machine somebody carries. A LaunchAgent also runs only while
someone is logged in — surviving a reboot unattended means a `LaunchDaemon` with a
`UserName` key, and FileVault still holds that until the disk is unlocked once.

### CI builds and tests on darwin now, so the claim has two platforms behind it

A second job in [`check.yml`](../.github/workflows/check.yml): `macos-latest`, `make test`
then `make build`. Four steps, no Node, no `make vendor`.

Not a matrix over the existing job — lint, ESLint, deadcode, the credential scan and the
bundle rebuild are platform-independent, so running them twice doubles the gate and returns
no new fact. And `make test`, not `make build` alone: compiling and linking the bindings is
not the failure worth catching, because sqlite-vec is registered *through* cgo and
`internal/db` drives it against a real database — a darwin difference shows up at run time
or nowhere.

It leaves `TestVendorTreeMatchesTheManifest` skipping with `vendor/ is empty`, deliberately:
a fresh checkout has only `.gitkeep` under `web/vendor`, the tree is identical on every OS,
and the Linux job checks it. That skip is covered there, not here — which is the distinction
rule 24 asks anyone reading a green run to make.

### Three cleanups

- `{cmd/server,cmd/ingest,internal/db,internal/ai,internal/rag,web}` existed as a literal
  directory tree at the repo root — a brace expansion that never expanded, run by a shell
  that does not do them. Zero files in it; verified before deleting.
- `README.md`'s *Now vs vNext* row for cross-OS self-host said "✅ shipped" on the strength
  of the documentation alone, while `make deploy` was Linux-only. The row now says what is
  true and names what it used to mean.
- The `build:` comment in the Makefile claimed `web/vendor/` "gets embedded", so
  `make vendor build` produced "an egress-free binary". It does not: the only embed of
  anything vendor-shaped is `web/vendor.sha384` in `web/assets.go`, and the tree is read from
  disk by `rendocs -base /vendor` and `make check-ui`. This cost something the same session it
  was found — a `make vendor` step was nearly added to the darwin CI job on the strength of
  it, which would have fetched every pinned asset to build a binary that never reads them.

---

## 2. Decisions, so they are not relitigated

| decision | why |
|---|---|
| **A second instance with its own corpus**, not a copy of the WSL one | it serves a different company's documents, so there is no shared source of truth to split. Invariant 1 holds per instance: each has its own `CORPUS_DIR` and derives its own DB. This is why no migration from `tonytlinux` is part of the plan |
| **Cloudflare Tunnel + Zero Trust**, not the tailnet | the tailnet is a *personal* one — `tonytlinux`, this Mac and a phone, one owner. Adding another company's team to it makes them members of it. Zero Trust puts the policy on *their* email domain and their own hostname, and they install nothing. This is the case the Deploy page's Cloudflare tab already describes |
| `AUTH_USER`/`AUTH_PASS` stay **empty** | Zero Trust authenticates in front of the binary. A second gate for one job is the duplication rule 17 exists to stop, and it would be the weaker of the two |
| **LaunchAgent, not LaunchDaemon** | the machine is also its owner's personal machine, so it is logged in when it is in use. No root, no `UserName`, and the upgrade path is a directory change plus one key if that stops being true |
| `cloudflared` gets **its own** supervisor (`cloudflared service install`) | it ships one. A hand-written plist for a vendor's daemon is a second copy of a fact the vendor already maintains |
| deploy directory **outside** the Obsidian vault | this repo lives inside one. `state/knowledge.db` is SQLite with WAL, and a WAL in a file-sync folder is a corrupt database; `.env` in one is a provider key leaving the machine. A *dev* DB is disposable, so the in-vault checkout stays fine for `make server` |

---

## 3. Open work — the host is not built

Everything below needs a secret or an account this session had no access to, so none of it
was done. It is a runbook, not a project.

```bash
git clone https://github.com/mega-hub-studio/mega-docs.git ~/srv/knowledge
cd ~/srv/knowledge && mkdir -p state
cp .env.example .env && chmod 600 .env      # AI_API_KEY · BA_PASS · ADMIN_PASS
#                                             DB_PATH=state/knowledge.db · PORT=8080
set -a; . ./.env; set +a; make live         # does this provider serve /embeddings?
make build && ./bin/ingest docs             # docs/ is gitignored: the corpus cannot reach
#                                             the public repo by accident
launchctl bootstrap gui/$UID ~/Library/LaunchAgents/dev.megadocs.knowledge.plist
curl -s localhost:8080/api/health           # {"ok":true,"writes":true}
sudo pmset -c sleep 0

brew install cloudflared                    # not present on this machine
cloudflared tunnel login && cloudflared tunnel create knowledge
cloudflared tunnel route dns knowledge <host>.<their-domain>
sudo cloudflared service install
#   then Zero Trust → Access → policy: emails ending in @<their-domain>
```

**A precondition that is not a step**: the hostname's zone must be in a Cloudflare account
with DNS rights, and Zero Trust enabled. Without it `tunnel route dns` fails — and it fails
at the second command, not the last.

Then the dev checkout gets `PORT=8081` in its `.env` and the two stop competing for the
port. Nothing else changes: `HEALTH` is derived from `PORT`.

### Verify before promising the URL to anyone

- **`/api/chat` is SSE.** Whether Cloudflare Tunnel passes it through unbuffered is
  measured, not assumed: ask one question through the public hostname and watch whether
  tokens arrive as a stream or as one block at the end. Buffering there is Cloudflare
  configuration, not an app bug — do not go looking in `internal/server`.
- `writes:false` from health means `BA_PASS` never reached the process. BA mode and import
  are both dead, and the UI says so before anyone types.
- `launchctl print gui/$UID/dev.megadocs.knowledge` is this platform's `systemctl status`;
  the log is wherever the plist's two path keys point.

### The corpus has no backup, and here it matters more

`2026-07-28-drop-corpus-sync.md` settled that keeping a copy of the corpus is a human's
job, and that decision is not reopened by this entry. But that one holds the owner's own
notes and this one holds a client's, so the asymmetry is worth naming: make `docs/` its own
git repo with a **private** remote once its folder layout stops moving.
`2026-07-27-wsl-deploy-and-import.md` §3a already verified how, including why the remote
must be SSH and not HTTPS.

