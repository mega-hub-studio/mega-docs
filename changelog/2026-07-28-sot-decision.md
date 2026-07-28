# 2026-07-28 — Decision: the Knowledge DB is the source of truth, and what that costs

Settled, after reading README-MEGA-DOCS.md properly. Two of its lines resolve the question
that had been open for three exchanges, and they resolve it *against* an earlier proposal of
mine, which is recorded here so nobody re-derives the wrong answer:

- **"Single Source of Truth: Only BA uploads via WebUI"** — the SoT is the *entry point*,
  not a git repository.
- **"Removed: Git sync"** — business documents live in **no git repo at all**.

So the split is:

| | source of truth | where | who can read it |
|---|---|---|---|
| platform code + published guide | `mega-hub-studio/mega-docs` | git, **public**, GitHub Pages | everyone — intended |
| enterprise knowledge | **Knowledge DB** | the instance's SQLite file | the instance |

The proposal I made earlier — a private `mega-corpus` repo — is **rejected**: it contradicts
"Removed: Git sync" and adds a second place the truth could live. Recorded because it looked
reasonable and is not.

What that also settles, definitively: **no business document is ever committed to
mega-docs**. That repo is public (`"private": false`, `"visibility": "public"`, Pages on),
and git keeps history, so a document committed there is public permanently even if a later
commit deletes it. Nothing in the vNext brief asks for that; it was a misreading of "SoT
repo mega-docs" on my part, checked before acting rather than after.

## The two prices, now chosen rather than stumbled into

Invariant 1 said `CORPUS_DIR` is the truth and `knowledge.db` is derived. Inverting it is
what the brief asks for, and it removes two properties that were load-bearing:

### 1. The schema needs a real migration runner — this is the blocker

"The schema has no migrations" was only safe *because* the database was derived: the upgrade
path for any change was `rm knowledge.db && ingest corpus`. With the DB as the truth there is
nothing to rebuild from, so:

- `CREATE TABLE IF NOT EXISTS` stops being sufficient. It already never reached an existing
  database for a new **column**, which was tolerable when the fix was a re-ingest and is not
  tolerable now.
- The Knowledge Model the brief lists (Sections, References, Tags, Categories, Relations,
  Version) is mostly *new tables* — those still arrive fine — but Document and Chunk will
  need new **columns** (version, status, category), and every one of those now needs a
  migration.

**This must be built before the corpus directory stops being written to, not after.** Doing
it in the other order removes the way back: the moment the DB is the only copy and the schema
cannot migrate, a schema mistake is unrecoverable.

### 2. Backup changes target, and there is currently none

"Put the documents folder in git" was the whole backup story. It stops applying. The DB
becomes the only copy of every uploaded document and every BA-confirmed answer, and today
nothing backs it up. A `sqlite3 .backup` on a timer, off-box, is the minimum before the first
real document is uploaded.

## Order of work, and why this order

1. **Migration runner.** Versioned, forward-only, applied at start, recorded in a table.
   Acceptance: adding a column to `documents` reaches an existing database, and a test proves
   it against a database created by the *previous* schema.
2. **DB backup.** Acceptance: a restore is exercised, not just a dump written.
3. Then, and only then: stop writing files, drop the CLI from every BA-facing path, and let
   `Upload` write straight to the Knowledge DB.
4. Roles as capabilities (`gate(role)` + `ADMIN_PASS`), the delete button on `.perm`, update
   by re-upload, tags/categories/version as new tables.

Steps 1 and 2 are unglamorous and they are the whole safety margin for step 3.

## Already true, and worth not re-doing

Checked line by line against the tree in the previous commit: OpenAI-only, no local LLMs, no
local embeddings, "never render raw HTML", and Markdown-Components → Renderer → NES all
already hold. The renderer chain is `marked → DOMPurify → dressTables → dressTaskLists →
linkCites → asDiagrams`, and three answer shapes already render from the NES library
(mermaid, `.table`, `.tasklist`).

Two items from "Removed" still need a decision rather than a commit — both in
`2026-07-28-vnext-collisions.md`: dropping BM25 would remove exact matching on error codes
and config keys over a Vietnamese corpus, and PDF/DOCX needs a parser this binary
deliberately does not carry.
