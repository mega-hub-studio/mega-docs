# 2026-07-28 — vNext: what is already true, what was cleaned, and the three collisions

The vNext brief (Platform-first, OpenAI-only MVP, WebUI-only, three roles, Markdown
Components → NES Renderer) was checked line by line against the tree rather than taken as
a work list. Most of the "Removed" column was already absent, one item was cleaned in this
commit, and **three items collide with load-bearing invariants** — those are decisions, not
tasks, and one of them changes how this system must be backed up.

## Already true, nothing to do

| brief says | reality |
|---|---|
| OpenAI only, no Gemini/Claude | one OpenAI-compatible client; no other SDK has ever been here |
| No local LLMs / local embeddings | never implemented — Ollama was nine prose mentions, not code |
| Never render raw HTML | `answer.js` sanitises with DOMPurify and builds components from *parsed values*, never by injecting model text as HTML |
| Reuse NES before creating components | mermaid, `.table`+`.table-wrap`, `.tasklist` all render from the library; app CSS owns one override, named in AGENTS.md |
| Markdown Components → Renderer → NES | the pipeline exists: `marked → DOMPurify → dressTables → dressTaskLists → linkCites → asDiagrams` |

## Cleaned here

`.env.example` no longer offers a menu of runtimes, and `internal/ai` no longer advertises
Ollama or LM Studio. What stayed, deliberately: the client still speaks the OpenAI wire
format, so Azure/Groq/OpenRouter work by base URL alone — that is a property of the
protocol, not a feature this repo maintains — and the keyless branch stayed, because
"send no Authorization header when there is no key" is correct for any keyless endpoint and
its comment now says that instead of naming a runtime. Removing it would have cut capability
without removing complexity.

## Collision 1 — the source of truth inverts, and migrations stop being optional

The brief's pipeline is `BA → WebUI Upload → Knowledge DB → embed → index`. Invariant 1 is
the opposite: `CORPUS_DIR` is the source of truth and `knowledge.db` is **derived**, which
is why `ingest` can rebuild it and why "the schema has no migrations" is safe — the upgrade
path for any schema change is `rm knowledge.db && ingest corpus`.

If the database becomes the source of truth, three things change at once and none of them
are in the Cleanup Summary:

1. **The schema needs real migrations.** You can no longer drop the database, so
   `CREATE TABLE IF NOT EXISTS` stops being sufficient and every new *column* needs a
   migration runner. Today a column costs one re-ingest; then it costs a migration.
2. **Backup changes target.** "Put the documents folder in git" stops being the answer;
   the database is the only copy. Note the corpus still has no remote — moving the truth
   into a file that also has no backup is worse, not better.
3. **`Version / Publish / Archive` become schema, not files.** That is fine —
   they are *new tables*, and `CREATE TABLE IF NOT EXISTS` does reach an existing database
   (unlike a column). So tags, categories, relations and versions cost no re-ingest.

The cheap middle path, if it is wanted: keep writing the uploaded file into `CORPUS_DIR`
(as `Upload` already does) *and* keep the DB derived. Then the WebUI is the only *entry*
point — which is what the brief actually asks for — while the rebuild path, the backup story
and the no-migrations rule all survive untouched. "No CLI import" then means the CLI is an
operator recovery tool, not a second way in.

## Collision 2 — "no hybrid pipeline" would remove exact matching

Retrieval today is vector KNN + BM25 fused with RRF, and the FTS5 tokenizer is
`unicode61 remove_diacritics 2` — chosen for Vietnamese. Dropping BM25 removes the half that
finds an error code, a config key or a rule id verbatim, and the BA guide's own advice is
that "half of retrieval is keyword matching, so an error code beats a paraphrase".

Worth separating two meanings of "one pipeline": *one path a question travels* (already
true — there is exactly one `Answer`), versus *one candidate generator*. The complexity the
brief wants gone is provider and import sprawl. Removing a retriever would measurably reduce
the accuracy that was named a critical property two days ago; it should not ride along on a
cleanup.

## Collision 3 — PDF/DOCX needs a parser this binary deliberately does not have

`TextExts` is `.md .markdown .txt`, and the README says why: keeping the parser out makes
"the documents are messy" a one-time cleaning step rather than a runtime failure mode. PDF
and DOCX in the brief means either a Go dependency (and its CVE surface) or an external tool
invoked at upload. Both are defensible; neither is free, and the choice belongs in the open.

## Order that does not require any of the above to be settled first

1. `gate(role)` + `ADMIN_PASS`, and `/api/health` reporting capabilities — roles as
   capabilities, ~15 lines, no dependency and no schema. Identity (accounts, sessions,
   argon2) is what per-user provider settings need, and that is the SaaS phase.
2. The delete button in `ImportPanel`, using the library's `.perm` recipe — the backend
   landed in 8fa63ce.
3. Update = re-upload the same path; `Upload` already replaces by path.
4. Tags / categories / version as new tables.

Still open and still blocking: one package manager (two lockfiles, CI runs `npm ci`), and
the corpus folder structure that the module/QA/FAQ filter derives from.
