# Commit message rules

Generate exactly one commit message in this shape. No preamble, no code fence, no emoji.

type(scope): summary [x.y.z]

Subject line ≤ 72 characters, including the version. Imperative mood, lowercase after
the colon, no trailing period. Say what the change *does*, never which files it touched.

## type → which digit moves

Pick one type. The type decides the version bump; never two types in one subject.

| type | use for | bumps |
|---|---|---|
| `refactor` | engine-level restructuring: a seam, an invariant, a schema, a layer contract | **x** if a caller/contract/DB shape changes, else `z` |
| `feat` | a capability that did not exist | **y** |
| `fix` | wrong behaviour made right | **z** |
| `perf` | same behaviour, measurably faster | **z** |
| `docs` | docs, comments, changelog entries | **z** |
| `test` | tests only | **z** |
| `build` | Makefile, Vite, committed `web/dist`, dependency pins | **z** |
| `ci` | workflows, gates | **z** |
| `chore` | config, tooling, housekeeping | **z** |
| `revert` | undoing a shipped commit | **z** |

`BREAKING CHANGE:` in a footer forces an **x** bump regardless of type.

## scope

One token, the lowest layer that holds the change. Use the directory name:

`server` `rag` `ai` `db` `config` `aitest` `ingest` `rendocs` `ui` `web` `docs`
`changelog` `spec` `make` `ci` `deps` `vscode`

Change spans several scopes → use the shared parent (`internal`, `web`) or omit the
scope entirely. Never list two scopes.

## version [x.y.z]

The commit subject is the **only** place the version lives — no VERSION file, no
duplicate in `package.json`. Read the current one, then bump exactly one digit and
reset every digit to its right:

```bash
git log -200 --format=%s | grep -oE '\[[0-9]+\.[0-9]+\.[0-9]+\]' | head -1
```

- **x** — engine-level: a public seam, an invariant, a DB migration that rewrites
  meaning, a layer rule, an API route's contract. `feat(db): …[2.0.0]` is legal when
  the feature breaks a caller; say so in a `BREAKING CHANGE:` footer.
- **y** — a new feature on top of the current **x**. Resets `z` to 0.
- **z** — fixes, docs, tests, build, chore on top of the current **x.y**.
- No version found in history → start at `[0.1.0]`.
- Merge commits keep the default merge subject and carry **no** version.

## body — only when it earns its place

Omit the body for a single-concern diff. Add it when the diff spans more than one
concern, changes an invariant, or the *why* is not obvious from the subject. Then:
blank line, then `- ` bullets wrapped at 72 characters, each naming a consequence,
not a file. Footers last: `BREAKING CHANGE: …`, `Refs: #123`.

## good

```
fix(rag): stop caching a whole miss, cache a partial answer [1.4.3]
feat(ui): pick a scope from the corpus tree before asking [1.5.0]
refactor(db): move the scope out of the cache signature into the key [2.0.0]
docs(changelog): record why the DB stays derived until backup exists [1.5.1]
build(deps): take 8bit-nes 0.7.3 and drop the workaround it replaces [1.5.2]
```

## reject

| bad | why |
|---|---|
| `update files and fix stuff [1.0.1]` | no scope, no effect, vague |
| `feat(ui,server): add scope [1.5.0]` | two scopes |
| `Fix: Added retry logic. [1.4.3]` | capitalised, past tense, trailing period |
| `feat(rag): add reranker` | version missing |
| `feat(rag): add reranker [1.4.3]` | `feat` must bump **y**, not **z** |
| `chore: bump version to 1.5.0 [1.5.0]` | the version is not a change |
| `refactor(db): rename Store methods, update tests, fix typo [2.0.0]` | three concerns in one subject |
