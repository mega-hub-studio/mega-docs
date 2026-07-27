# 2026-07-27 — Linting the Go side: from 591 findings to zero, and a config that explains itself

Ran the vendored `golang-lint` skill's recommended `.golangci.yml` over the tree, then
did the part that actually matters: decided which of its 591 findings were facts.

## The gate

`.golangci.yml`, `make lint`, `make lint-fix`, and `make check` now runs it. CI installs
golangci-lint instead of staticcheck, because staticcheck runs inside it; `make dead`
keeps deadcode alone, which is the one check golangci-lint has no equivalent for
(whole-program reachability from the two mains).

**The tree is at zero findings.** That is the property worth protecting: 591 issues is
not a lint report, it is wallpaper, and a gate that always shouts is one everybody learns
to scroll past. Five linters are off and each has its reason written beside it in the
file:

| off | why |
|---|---|
| `wsl_v5` | 304 findings, all "put a blank line here". This repo's comments carry the structure. |
| `paralleltest` | 116 findings. Every test owns a real SQLite file; the suite runs in under a second. |
| `noctx` | 33 findings, all `database/sql` calls in `internal/db`. See below. |
| `dupl` | test setup repeats on purpose — a shared fixture fails for reasons not in the test. |
| `gofumpt` | it classifies a dotless module path as stdlib, so it merged every `knowledge-engine/…` import into the std group. |

`gocyclo` is at 16 rather than 13: `SafePath`, `SplitMarkdown` and `Engine.Answer` sit at
14–16 and every branch in them is a rule the tests name one by one. Splitting them would
move the branches, not remove them.

## What the linters found that was real

- **`log.Fatal` skipped `defer store.Close()`** in both binaries (gocritic
  `exitAfterDefer`). `os.Exit` runs no deferred call, so a WAL database never got its
  last checkpoint. Both mains are now three lines that call `run() error` — which is also
  what dropped `ingest`'s cyclomatic complexity, once `collect()` came out of it.
- **A dropped `filepath.WalkDir` error** meant a folder the process could not read
  produced `0 files` with no reason. It now names the path — "empty corpus" and
  "permission denied" were indistinguishable.
- **`rows.Err()` was never checked** (rowserrcheck ×3). A truncated iteration returns the
  rows it managed to read, which looks exactly like a corpus with fewer matches. Two of
  the three now return the error; the third — the keyword leg — reads it and discards it
  deliberately, with the reason written where it happens.
- **`Store.Search` at complexity 19** became four named steps: `vectorCandidates`,
  `keywordCandidates`, `fuse`, `hits`. The extraction is what made the best-effort
  keyword leg *visible*: it returns no error, and the comment says why.
- **Corpus files were written 0644 in a 0755 directory.** A confirmed answer is a
  business document written by the service, for the service. Now 0600/0750.
- **`"approved"` was a bare string in a Go comparison** — a typo there is a boost that
  silently never applies. It is `statusApproved` now; the SQL keeps its literal, because
  there it is SQL.
- Plus the small true ones: `strings.Cut` over `IndexByte`, `slices.Contains` over a
  loop, `errors.New` where `fmt.Errorf` had nothing to format, `http.MethodGet` over
  `"GET"`, `for range n`, and every remaining ignored error written as `_ =` at the call
  site so the decision is visible.

## Two suppressions, both with the reason attached

- `//nolint:gosec` on the two scope-filter queries. G202 flags the concatenation; the
  fragment is constant SQL from `scopeFilter` and every value — including the scope — is
  a bound parameter.
- `//nolint:nilerr` on `Engine.History`. It returns an empty list when the corpus
  signature cannot be read, which the doc comment already said: the panel is a
  convenience, the answer is the product.

## Documentation the linter was right about

Every package now has a `// Package …` comment, and the exported symbols revive named
(`ai.Client`, `ai.New`, `ai.Msg`, `config.Config`, `config.Load`, `db.Store`,
`db.Store.Close`, `db.Store.Ticket`, `rag.Engine`, `rag.New`, `web.Dev`, `web.Deploy`)
have one that says what they are for rather than restating the signature. The blank
`go-sqlite3` import says what it is for. `godoc` on this module is now worth reading.

## Deliberately not done

- **Threading `context.Context` through `internal/db`** (the 33 `noctx` findings). Every
  query is a local SQLite read measured in microseconds; the cancellable work is the
  provider call, which already takes a `ctx`. The change would touch every method of the
  store for no measurable gain. Turn `noctx` back on the day a query is slow enough to
  want cancelling, or the store stops being a local file.
- **Renaming the module** from `knowledge-engine` to its repository URL. It would fix
  gofumpt's heuristic *and* the stale product name, and it rewrites the import line of
  every file — a decision of its own, not a formatting side effect.

## Verified

`make check` clean (tests, gofmt, vet, golangci-lint at zero, deadcode, secret scan),
and the refactored retrieval driven end to end: the same question unscoped cites the
controls document, scoped to `booking/calendar/sidebar` cites only the quick-panel one.
