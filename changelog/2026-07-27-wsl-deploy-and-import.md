# 2026-07-27 — First deployment (WSL host) and the document import surface

Two things happened: the engine was deployed for a team, and it grew a way to get
documents into it from the browser. Everything below is the state a next session
inherits.

---

## 1. What is running

A WSL2 host (Ubuntu 24.04, systemd as PID 1), reachable on the owner's tailnet.

| | |
|---|---|
| URL | `https://tonytlinux.taile61671.ts.net:8443` |
| Deploy dir | `/opt/knowledge` — mode 750, owned by `tonytlinux` |
| Binaries | `/opt/knowledge/bin/{knowledge,ingest,corpus-sync}` |
| Corpus (`CORPUS_DIR`) | `/opt/knowledge/docs` — **its own git repo** |
| Units | `knowledge.service` · `corpus-sync.path` · `corpus-sync.timer` |
| Build tree | `~/msh/mega-docs` (this repo) — **not** the deploy dir |
| Provider | OpenAI for both: `gpt-4o-mini` chat, `text-embedding-3-small` 1536 |

`EMBED_BASE_URL` and `EMBED_API_KEY` are deliberately **empty**: one provider serves
both endpoints, so splitting them would be two places to get wrong. Secrets live in
`/opt/knowledge/.env` (mode 600) — `AI_API_KEY` and `BA_PASS`. Never copy either
into this repo.

Health must report `{"ok":true,"writes":true}`. `writes:false` means `BA_PASS` did
not reach the process, and BA mode plus import are both dead.

### Deploying a change

The deploy dir is not a git clone, so the four lines on the Deploy page do not apply
verbatim:

```bash
cd ~/msh/mega-docs && git pull origin main
make check && make build
sudo systemctl stop knowledge
cp bin/knowledge /opt/knowledge/bin/knowledge.new && mv -f /opt/knowledge/bin/knowledge.new /opt/knowledge/bin/knowledge
cp bin/ingest /opt/knowledge/bin/
sudo systemctl start knowledge && curl -s localhost:8080/api/health
```

`mv`, not `cp`, for the server binary: the running process holds the inode and `cp`
fails with `Text file busy`.

---

## 2. What was built

### Import (`POST /api/documents`)

`web/app/upload.js` · `internal/server/documents.go` · `internal/rag/upload.go`

Multipart, field `files` (repeatable) plus an optional `dir`. Behind the **same
`BAPass` gate as a confirm** — the app has no accounts, so an open import lets
anyone who reaches the port rewrite what everyone reads. `Deps.Docs` is nil-able the
way `Deps.Know` is, so the route disappears rather than half-works.

Decisions worth not relitigating:

- **Partial success is reported, not hidden.** Eight files where one is a PDF →
  seven indexed, the eighth named. `200` if anything landed, `400` if nothing did,
  and the `400` body still lists every file, so the UI renders one list either way.
- **Folders are kept** (`rag.SafePath`). They are the scope a reader browses, so
  flattening a path destroys the structure the tree UI needs. Validation is
  structural and per segment — not a blocklist: no `..`, no hidden segment, no
  absolute path or drive letter, `MaxDepth` 4 segments, and `qa/` is refused so an
  import cannot impersonate an answer a BA vouched for.
- **The chosen `dir` is joined *before* validation**, so `../` in the folder box is
  exactly as harmless as `../` in a file name.
- Re-importing the same path updates that document in place — the same identity rule
  `cmd/ingest.docPath` uses, so the CLI and the browser cannot disagree.

### Three defects found by driving a real browser, and fixed

Reported as "BA mode cannot upload and there is no progress bar". The upload path
itself was fine — a headless Chrome over CDP indexed a file through the UI on the
first try. What was broken was everything around it:

1. **A wrong password was a trapdoor.** `unlock()` stored the password without
   checking it, so the import card appeared, and the first upload turned the 401
   into `unlocked = false` — the card vanished and the unlock form came back. That
   is indistinguishable from "upload is broken". `unlock()` now verifies against the
   gate first (an empty multipart POST to `/api/documents`, which changes nothing)
   and shows the failure inline in the form.
2. **`dragenter` never called `preventDefault`.** Only `dragover` did, which Chrome
   tolerates and the spec does not — the element was not a valid drop target.
3. **No progress indicator at all.** Fixed with the design system's own `.spinner`
   and `.pbar`, and by uploading **one file per request** so the bar is determinate.
   A single POST for the batch could only be animated by guessing, and a bar that
   invents its position claims "nearly done" while the last file has not started.

Verification is `scratchpad/verify.mjs`-style CDP driving, not a click-through: the
three checks are `defaultPrevented` on all three drag events, `--fill` moving
0% → 66.7% with the text going "1 of 3" → "3 of 3", and a wrong password leaving
`importCardLeaked: false`.

### The system prompt, and two cache-policy bugs behind it

The prompt gained four rules, each answering a failure the old five did not cover:
partial coverage (answer the part that is covered, name the part that is not),
disagreeing sources (say so and cite both), citation discipline (never a number
that is not in the CONTEXT), and identifiers verbatim (a Vietnamese answer over
English documents must not translate `CORPUS_DIR` or a command). The persona also
stopped saying "engineering team" — the corpus now spans business, product and
support, and BA/PM are the primary readers.

Changing it surfaced two bugs of the same family as the `CHAT_MODEL` one:

1. **The prompt was not in the cache signature.** Edit the rules and every cached
   answer keeps claiming instructions it was never given. `sig()` now appends
   `promptSig`, a hash of the constant — computed, not a version number someone has
   to remember to bump.
2. **A partial answer was never cacheable.** The skip rule was
   `strings.Contains(reply, NoAnswer)`, and a model asked to name the uncovered part
   reaches for exactly that sentence however firmly the prompt reserves it —
   measured, on gpt-4o-mini, after two attempts at wording it away. So the most
   expensive answers the engine produces were the only ones it never remembered.
   `isMiss` now matches a reply that *is* the sentinel, and caching a partial answer
   is safe because the signature carries the corpus: the day the missing document
   arrives, it is invalidated with everything else.

The lesson worth keeping: a prompt cannot enforce an invariant the code depends on.
Reserve the sentence in the prompt for readability, but recognise a miss in code.

### Corpus structure

`/opt/knowledge/docs/README.md` is the convention and is itself indexed, so the
corpus answers questions about its own layout. Top level: `business/` `product/`
`engineering/` `support/`, plus `qa/` which belongs to the app.

The rule that matters: **level 1 is the scope a reader will click**, folder names are
ASCII kebab-case (two spellings of one domain are two scopes and split the corpus
silently), file names may be Vietnamese, three folders deep maximum.

### Git sync (`scripts/corpus-sync.sh`)

A `.path` unit watching `docs/` and `docs/qa/` commits whatever lands there, from
either write path, and pushes if a remote exists. Event-driven rather than a timer,
because there is nothing to do until a file changes — the Deploy page's argument
against a re-index schedule applies here too.

Two failures found by testing it, both fixed, both worth remembering:

1. **A `.path` unit stops watching while the service it triggered runs**, so a file
   written during that window raises no event and waits for the next unrelated
   change. The script now loops until the tree is clean, and `corpus-sync.timer`
   (15 min) is the backstop. That timer is affordable precisely because it costs a
   `git status`, not an embedding call.
2. An earlier version piped `curl` into a counter, so **the pipeline's status was
   the counter's**: a rejected key was reported as "0-dim vectors". Same trap
   `scripts/smoke.sh` warns about.

### `scripts/switch-embed.sh`

Moves embeddings to another provider: validates the key against the new endpoint
**before** dropping the index, then re-ingests. `DIR=/opt/knowledge SERVICE=knowledge
make switch-embed`.

---

## 3. Open work

### a. The corpus has no remote — set this up first

`/opt/knowledge/docs` is a local git repo with no `origin`, so nothing is backed up
off the box and "expose the SoT to the cloud" is not done.

```bash
cd /opt/knowledge/docs && git remote add origin git@github.com:<org>/<corpus>.git
git push -u origin main
```

**The remote must be private.** `mega-hub-studio/mega-docs` is public (anonymous
`git-upload-pack` returns 200, and Pages serves from it) — internal business
documents pushed there are published. The corpus repo is a *different* repo from
this one, on purpose.

Done when: a BA confirm or a browser import appears in the remote within ~15 minutes
with no human action.

### b. Tree UI + scoped retrieval

The requested end state: a folder tree in the app, click a branch, ask a question
scoped to it.

- **The tree is a component, not a build.** 8bit-nes 0.7.1 already ships
  `<nes-tree>` — "hierarchical folder/file list, expand/collapse, single or multiple
  selection, full keyboard nav, data via child JSON". Check its `llms.txt` before
  writing any of it; there is also `<nes-code-tree>` if a viewer pane is wanted.
- The data already exists — `GET /api/corpus` returns full paths, so the tree is
  derivable client-side with no API change. `web/app/upload.js` already has
  `folders()` doing exactly this collapse for the import picker.
- The real work is retrieval: `POST /api/chat` needs a `scope`, and hybrid search in
  `internal/db` needs a path-prefix filter that applies to **both** the vector and
  the FTS side before RRF fuses them. Filtering after fusion returns fewer than
  `TOP_K` results and silently degrades the answer.
- Not started deliberately: with only three documents indexed there is nothing to
  verify a scope filter against.

### c. The corpus is still mostly empty

Three documents, all about this project. No real business or engineering material has
been loaded yet — the owner has that. Until then, any retrieval-quality judgement is
about the wrong corpus.

### d. Fork divergence

`internal/server/server.go`, `web/index.html`, `web/app/app.js` and `styles.css` now
carry local changes, and upstream moves fast (2.8k lines landed in one hour on the
day of this deployment). Upstream has already adopted `scripts/switch-embed.sh` from
this fork, so proposing the import surface upstream is likely cheaper than carrying
it. Expect conflicts in those four files on every pull.

---

## 4. Host quirks that cost time

| Quirk | Workaround |
|---|---|
| `proxy.golang.org` unreachable from this network (github and `sum.golang.org` are fine) | `GOPROXY=direct` on every `go` command, or `go env -w GOPROXY=direct` once |
| `make live` cannot see the repo `.env` — `go test ./internal/ai/` runs with CWD in the package dir, and `config.Load()` reads `./.env` | `set -a; . ./.env; set +a; make live` |
| Tailnet root `/` is taken by another service on `127.0.0.1:9119` | published on `--https=8443`; `--set-path` is not an option, the app serves absolute `/app/…` URLs |
| Changing `CHAT_MODEL` serves stale answers | the answer cache keys on the normalised question and invalidates on `corpus_sig` only — the model is not in the key. `sqlite3 knowledge.db 'DELETE FROM answers'` after a model change |
| systemd only runs while the WSL distro is up | the host has a separate always-on mechanism for that; `knowledge.service` rides on it |

The cache one is a genuine upstream gap, not a local misconfiguration: folding the
chat model into `corpus_sig` would fix it at the source.

---

## 5. Verifying

```bash
make check                                   # the gate — never skip it
curl -s localhost:8080/api/health            # {"ok":true,"writes":true}
curl -s localhost:8080/api/corpus            # what is indexed, with full paths
cd /opt/knowledge/docs && git log --oneline  # every confirm and import, in order
systemctl is-active knowledge corpus-sync.path corpus-sync.timer
```

The import path end-to-end, without a browser (`$PASS` from
`/opt/knowledge/.env`, never from a command line in a shared transcript):

```bash
curl -sS -X POST https://tonytlinux.taile61671.ts.net:8443/api/documents \
  -H "X-BA-Pass: $PASS" -F "dir=business/pricing" -F "files=@note.md"
```
