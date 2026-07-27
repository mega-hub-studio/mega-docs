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
| Deploy dir | `/opt/knowledge` — **a clone of this repo**, mode 750, owned by `tonytlinux` |
| Corpus (`CORPUS_DIR=corpus`) | `/opt/knowledge/corpus` — its own git repo, nested and locally excluded |
| Index (`DB_PATH=state/…`) | `/opt/knowledge/state/knowledge.db` — derived |
| Units | `knowledge.service` · `corpus-sync.path` · `corpus-sync.timer` |
| Provider | OpenAI for both: `gpt-4o-mini` chat, `text-embedding-3-small` 1536 |

**Three things, three lifecycles — which is why the deploy directory has three
places and not one.** The code is public and tracks a fast-moving upstream; the
corpus is private and changes at the business's pace; the index is derived and
belongs to neither. Putting the corpus in the code repo publishes it (that repo
answers an anonymous `git-upload-pack` with 200), makes every upstream merge touch a
tree containing company documents, and turns each BA confirm into a machine commit on
the code history.

`corpus/` and `state/` are hidden from the outer checkout via
`.git/info/exclude` — local to the deploy, so the repo itself needs no change.

`EMBED_BASE_URL` and `EMBED_API_KEY` are deliberately **empty**: one provider serves
both endpoints, so splitting them would be two places to get wrong. Secrets live in
`/opt/knowledge/.env` (mode 600) — `AI_API_KEY` and `BA_PASS`. Never copy either
into this repo.

Health must report `{"ok":true,"writes":true}`. `writes:false` means `BA_PASS` did
not reach the process, and BA mode plus import are both dead.

### Deploying a change

Exactly the four lines on the Deploy page, because the deploy directory *is* a
checkout:

```bash
cd /opt/knowledge && git pull origin main
make build
sudo systemctl restart knowledge
curl -s localhost:8080/api/health      # {"ok":true,"writes":true}
```

Two consequences worth knowing. Only **committed** code can be deployed — there is no
copy step to smuggle a working tree through, which is the point. And the service
cannot write to what it runs: `ReadWritePaths` lists `corpus/` and `state/` only, so
`make build` (run by a human) writes `bin/`, and the process cannot.

Building while the old binary runs is fine; `systemctl restart` picks up the new
inode. `cp` over a running binary would not — it fails with `Text file busy`, which is
why the previous layout needed `mv`.

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

Top level: `business/` `product/` `engineering/` `support/`, plus `qa/` which belongs
to the app.

The convention itself lives in `/opt/knowledge/.system-docs/corpus-convention.md` —
**outside `CORPUS_DIR`, so it is not indexed**. It used to be `docs/README.md` and was
indexed, which is exactly how "list every document and the folder structure" became an
answerable question. Nothing describing the app is in the corpus any more; grounding
does most of the domain lock, and the prompt rule is the second layer for a document
that tries prompt injection.

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
   change. The script now loops until the tree is clean.
2. **A `.path` unit is not recursive**, which only became visible once the corpus
   became a tree: `PathModified=/opt/knowledge/corpus` fires for a file written
   directly in it and misses `corpus/support/faq/nested.md` — where imports land.
   Enumerating folders would break the first time a BA makes a new one, so
   `corpus-sync.timer` was promoted from 15-minute backstop to the **guarantee** at
   two minutes, with the watcher kept as the fast path. Measured: a nested import is
   committed ~115s later. Affordable because waking to find nothing costs a
   `git status`, not an embedding call.
3. An earlier version piped `curl` into a counter, so **the pipeline's status was
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

### b. Tree UI + scoped retrieval — **done**, see `2026-07-27-scoped-retrieval.md`

Built once the corpus had something to verify against. The plan below held, including
the pre-filter warning; what it did not anticipate was the cache key, which the same
question in two folders breaks if the scope is not part of it.

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
  verify a scope filter against. *(That condition lifted — see below.)*

### c. The corpus has real material now

Five dev-handoff documents landed under `docs/booking/**` (471 sections, three folders
deep), which is what made the scope filter verifiable. Retrieval quality can now be
judged, but only about booking — business, product and support are still empty.

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
| ~~Changing `CHAT_MODEL` serves stale answers~~ — **fixed upstream**, no workaround needed | `Engine.sig` now appends the chat model *and* a hash of the system prompt, so changing either invalidates the cache by itself. The manual `DELETE FROM answers` is no longer required |
| systemd only runs while the WSL distro is up | the host has a separate always-on mechanism for that; `knowledge.service` rides on it |

The cache one was a genuine upstream gap rather than a local misconfiguration, and it
was fixed where it belonged — in the signature, not in a runbook step someone has to
remember.

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
