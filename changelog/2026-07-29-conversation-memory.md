# 2026-07-29 — Conversation memory ships, and the vNext status list was stale

Built the thing `2026-07-28-memory-and-external-search.md` designed, with both of its traps
handled and one interaction it did not anticipate. External search is still **not** built —
that entry's recommendation was to do memory first and completely, and this is that.

## The status list this session started from was wrong in four places

Worth recording, because the list was carried over from a previous session and would have
sent the next one to build things that already exist:

| the list said | actually |
|---|---|
| **1. DB backup + exercised restore — the last blocker** | **void.** `2026-07-28-no-backup.md` dropped the backup requirement outright, and `README.md`'s *Now vs vNext* already reads 🟡 next with nothing in front of it. The list was written against `sot-decision.md`, which that entry reverses |
| **3. `gate(role)` + `ADMIN_PASS`** | **shipped.** `ADMIN_PASS`, `GET /api/settings`, `/#/admin`, `AdminScreen.vue`, and the Deploy page's `#admin` section |
| **4. Delete button in `ImportPanel` using `.perm`** | **shipped.** `ImportPanel.vue` renders the `.perm` confirmation; the file goes to `docs/.trash/` |
| **5. Update = re-upload the same path** | **shipped.** `Upload` replaces by path |

Also on the list: *"README.md + CLAUDE.md still describe the derived-DB world"*. They do, and
that is **correct** — the DB *is* still derived, because the inversion is the work that has
not been done. There is nothing to rewrite until it lands. Rule 19 cuts the other way here:
the brief is not the spec, so documenting today's code is not documenting a lie.

So the real next item was memory, which is what got built.

## What shipped

`internal/rag/memory.go` is the whole seam: `Turn` (the wire shape of one exchange),
`replay` (thread → provider messages), and `standalone` (the rewrite). `Answer` gained six
lines and no new concepts.

- **Trap A, the cache, handled by exclusion.** `cacheable` is one named boolean —
  `sigErr == nil && len(turns) == 0` — read by both the lookup and the store, because a
  read and a write that disagree about what may be cached is the bug itself. A follow-up
  therefore touches no row in either direction. The alternative (hashing the history into
  the key) stays unbuilt: it makes the hit rate on follow-ups approximately zero anyway, so
  it would be complexity bought with no saving.
- **Trap B, retrieval, handled by a rewrite.** One cheap completion turns
  "còn bước 2 thì sao?" into a standalone question, and *that* is what gets embedded and
  handed to BM25. A rewrite failure returns the original wording rather than an error — the
  typed question is a worse query, not an invalid one.
- **The rewrite's tokens are counted.** `usage` folds in what the rewrite spent, once,
  before every return. An unmeasured cost printed as zero is the trap `CLAUDE.md` already
  names; a turn that quietly cost two completions and reported one is the same lie.

## The interaction the design note missed: small talk and vagueness

`smallTalk` intercepted `tooVague` — "cái đó", "sao", "how", a bare "?" — before retrieval,
and asking those back was right when a question arrived alone. With a thread behind it, it is
the exact failure memory exists to remove: the turn above carries the content word this one
is missing, so "could you be more specific?" answers a question nobody asked.

So `smallTalk` takes `inThread` now, and it changes **exactly one** verdict: `tooVague`
falls through to the rewrite. Greeting, thanks, identity, capability and `runtimeMeta` are
untouched — "cảm ơn" is thanks on the tenth message as much as on the first, and routing it
to retrieval would buy a completion to say you are welcome.

The classification moved into `smallTalkKindOf` to keep that readable. No behaviour rides on
that split.

## How much thread, and who decides

Three exchanges, chosen in `web/ui/src/composables/conversation.js` (`RECALL_TURNS`), and
answered turns only — a question whose answer errored or was stopped would tell the model it
went unanswered. Two facts, two owners, neither duplicated:

- *which* turns are relevant → the client, which owns the thread and pays to send it
- *how big* a request may be → the server, `maxAsk`, now 64 KiB (was `maxQuestion`, 8 KiB;
  the old name became a lie the moment a thread rode along with the question)

## Verification

`make check` green, unfiltered — the whole log read, not grepped. Every stage ran: `secrets`,
`vet`, `lint` (**0 issues**), `lint-js`, `dead`. Two things worth knowing about it:

- `TestBuiltUIMatchesItsSources` fired first time and was right: `make ui` before trusting a
  built binary, every time.
- `Engine.Answer` was already at `gocyclo`'s ceiling of 16, and the two cache gates pushed it
  to 17. The fix was extracting `serveCached` — which removes two decision points and reads
  better — not raising the limit. `.golangci.yml`'s note that these four functions sit at
  14–16 is still true, so it was left alone.

The acceptance test is the one the design note asked for:
`TestAFollowUpIsRewrittenForRetrievalAndNeverCached` (in `qa_test.go`, which owns cache
identity). It was confirmed **red** before it was green — with the rewrite short-circuited it
reports `a follow-up bought 1 completions, want 2`. It asserts against the fake provider's
own call log: two completions, one embedding call, the embedded text being the rewrite and
not the pronoun, the answering call carrying the prior turns, and the follow-up's own words
still cold in the cache afterwards.

`aitest.Provider` gained `Messages()` for the last of those. `Chats()` records only each
request's last user message, so nothing could see whether a conversation reached the model —
which is the entire feature.

## Also closed: two lockfiles, one manager

`web/ui/pnpm-lock.yaml` is **deleted**, and its `make secrets` exclusion with it — an
exclusion for a file that no longer exists is the dead config in the gate rule 24 calls a
lie. Nothing was decided here that was not already decided: CI installs with `npm ci`, the
committed bundle is built from `package-lock.json`, and `CLAUDE.md` already lists the vendored
`pnpm` skill as one that does **not** apply. `vnext-collisions.md` had it as "still open and
still blocking"; the Makefile comment said "pick one manager; until then this only stops the
scan from crying wolf". Picked.

Worth knowing, and possibly why it mattered: rebuilding the bundle after `npm ci` renamed
**every** mermaid chunk, not just the app's. Two consecutive `make ui` runs are byte-identical
and `build.json` is stable, so the tree is now consistent with `package-lock.json` — which is
what CI rebuilds from and diffs. A dist built from a differently-resolved tree is precisely
the failure that comment predicted.

## What was deliberately not done

- **The retrieval diagram was not changed.** `web/retrieval.mmd`'s own header says keep it
  shallow, and that is a measured phone constraint: a spotlit node in a diagram taller than
  the viewport scrolls off screen. An honest drawing of the follow-up path costs two ranks
  (the decision, then the converge back), taking 9 to 11. The fact is in the section's
  `datalist` as *rewrite · a follow-up only* instead, alongside the `cached?` entry it
  qualifies — which is where that page's per-node detail already lives. Revisit if the
  walkthrough gets a way to page a taller diagram.
- **No history hash in the cache key** — see Trap A above.
- **External search** — unchanged from yesterday's entry: off by default, labelled,
  signature must carry it, and it sends internal questions to a third party. Still a
  decision, not a sprint.

## Environment note for the next cloud session

`golangci-lint.run` is **blocked by egress policy** in this environment (403 on CONNECT), so
`make lint`'s installer cannot run — the baseline `make check` failed at the lint stage before
any code was touched. `proxy.golang.org` is allowed, so
`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2` produces the
pinned version and `lint-deps` then finds it and skips its download. **The Makefile was not
changed**: upstream recommends the binary install and `CLAUDE.md` says why. This is a local
workaround for one sandbox, not a new convention.

`npm i -g pinchtab` does work here (the registry is reachable and Chromium is pre-installed),
so `make check-ui` and `make check-wt` can run in this environment rather than skipping.
