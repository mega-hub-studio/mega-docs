# 2026-07-28 — Clearing what was left half-done

Four things were outstanding rather than undecided. All four are closed; the two that
changed behaviour are described in enough detail to undo.

## 1. The host is clean — corpus-sync units removed

`scripts/corpus-sync.sh` was deleted earlier today, but this box still had its units
**enabled**, pointing at a file that no longer existed. Done now:

```
systemctl disable --now corpus-sync.path corpus-sync.timer
rm -f /etc/systemd/system/corpus-sync.{path,timer,service}
systemctl daemon-reload
```

`knowledge.service` is untouched and still `active enabled` — it never depended on the sync.
**Nothing backs the corpus up automatically. That is the accepted state**, recorded in
`2026-07-28-drop-corpus-sync.md`, and the Deploy page says so in both languages.

## 2. `DELETE /api/documents` was reachable by nobody

The route landed in 8fa63ce behind the BA gate, with tests, and **no UI ever called it** — a
shipped endpoint with no user, which is the most expensive kind of dead code: it looks
finished. Now wired, in the layer each part belongs to:

| | |
|---|---|
| `lib/upload.js` | `remove(path)` — `DELETE`, `X-BA-Pass`, `WrongPass` on 401/403, exactly like `sendOne`. Each path segment is encoded separately so `/` survives as a separator while a space or `#` does not |
| `composables/importer.js` | `pending` / `removing` / `askRemove` / `cancelRemove` / `confirmRemove`. Every branch is here. One document pending at a time — a queue of pending deletions is a way to confirm the wrong one |
| `ImportPanel.vue` | a collapsed `<details>` listing indexed paths with a REMOVE button, and the library's **`.perm`** recipe as the confirmation. Markup only |

Two decisions worth not re-deriving:

- **Two steps, never one.** Import is additive and a mistake costs an ingest; removal takes
  a source away from every future reader, and the buttons sit in a list of paths that differ
  by one word. `.perm` exists for precisely this — the target shown verbatim, one decision.
- **The toast names the trash path.** `rag.Remove` moves the file to `docs/.trash/` rather
  than unlinking it, so "deleted" would be a lie and `mv` is the undo. Worth saying out loud
  now that nothing else backs the corpus up.

**New test** — `TestRemovingADocumentIsGated/an_encoded_name_is_unescaped_before_the_engine_sees_it`.
It asserts the half of the contract the client depends on: ServeMux unescapes `{path...}`, so
`business/Q3%20pricing%20%232.md` reaches the engine as `business/Q3 pricing #2.md`. Without
it, `Q3 pricing.md` is undeletable through the UI while every test with a tidy file name
passes.

## 3. Rule 11 had a hole, and something was already through it

`ChatTurn.vue` decided two mid-stream rendering rules inside `<script setup>`
(`streaming ? [] : citations`, `!streaming && diagramsReady`) while
`TestComponentsHoldNoLogic` reported clean: `reBranch` only matches a line *starting* with
`if`/`for`/`while`/`switch`, and a ternary is an expression.

- The rules moved to `lib/answer.js` as `turnHtml(turn, diagramsReady, srcId)` — they are
  decisions about what the HTML may contain, which is that file's job. `ChatTurn.vue` is one
  line now.
- `reTernary` added to `web/frontend_test.go`, applied to `components/*.vue` only.
  `[^.?]` after the `?` keeps `a?.b` and `a ?? b` out — those are value access, not a
  decision. Verified it matches the old ChatTurn line and none of the safe forms, so the
  enforcer is not vacuous.

The template may still branch freely: `{{ importing ? 'IMPORTING…' : 'CHOOSE FILES' }}` is
how a template asks a question. This is about `<script setup>` deciding.

## 4. Three rows the Now vs vNext join was missing

Not gaps in the code — gaps in the **table**, which is worse, because the table is what the
next agent reads instead of checking. Added: BA verbs (create/update/delete now ship, the
other five do not), Response Format (Related Documents and Suggested Actions have no field in
`rag.Reply`), Knowledge Model (five of nine entities have no table; all five are new
*tables*, so they cost no re-ingest).

## Checked and found clean, so nobody re-checks

- `changelog/README.md` is a criteria document, not an index — it cannot go stale from a file
  count, and its criteria still match how these files are written.
- `AGENTS.md`'s 8bit-nes pin matches `web/vendor.sha384` (`0.7.3`), and a test already
  enforces it.
- Every `.PHONY` name in the Makefile has a real target — the `switch-embed` deletion left
  nothing dangling.
- No residue of `SITE_URL`, `ASSET_BASE`, `corpus-sync`, `switch-embed`, Ollama, `11434`,
  `nomic-embed` or `web/app/` outside `changelog/` and the comments that explain their
  absence. The two remaining `app.js` mentions are deliberate: a dated historical note, and
  `server_test.go` asserting `/app/app.js` returns 404.
