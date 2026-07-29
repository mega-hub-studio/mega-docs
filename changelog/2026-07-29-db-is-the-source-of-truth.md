# 2026-07-29 — The Knowledge DB is the source of truth, and the BA has a library

Invariant 1 is inverted. `CORPUS_DIR` was the source of truth and `knowledge.db` was derived;
the row is the document now, the WebUI is the only way one enters, and `internal/rag` does not
open a file.

## What the inversion actually deleted

Worth listing, because the value here is subtraction:

| gone | why it existed | why it does not now |
|---|---|---|
| `Engine.writeDoc` | one seam wrote every corpus file — imports and confirmed answers | there is no file; `ingest()` writes the body with the row |
| `rag.Options.CorpusDir`, `Engine.corpusDir` | an unset directory disabled writes, so a confirm could not produce an unreproducible index | nothing to configure before a BA can save |
| `rag.TrashDir` and the `.trash/` move | a hard delete would have destroyed an original | `deleted_at`: chunks go, text stays |
| `db.DeleteDocument` | the hard delete behind Remove | replaced by `RemoveDocument`, which is the soft one |
| `TestConfirmWithoutACorpusDirectoryFailsBeforeIndexing` | a confirm needed somewhere to write | there is nowhere to be without |
| `TestConfirmedAnswerIsReproducibleByIngest` | "a second engine, given only the directory, arrives at the same corpus" — what made the DB disposable | it is not disposable, and a test asserting otherwise is a lie in the gate |
| `engineIn(t, p, corpusDir)` | the test harness variant for looking at what a confirm wrote to disk | nothing writes to disk |

Two tests were **deleted rather than adapted**. Adapting them would have kept a green check
over a property the code no longer has, which is worse than no check — the note where they
stood says so, and names what replaced them.

## The shape that carries it

One migration (`id: 1`), five columns on `documents`:

- **`body`** — the document itself. This is the inversion; everything else is attributes.
- **`title` · `alias` · `kind` · `description`** — what a BA files it under, because retrieval
  reads the text and a *person* searches by half-remembered names six months later. `alias` is
  the one that earns its place fastest: "rate card" finds `business/pricing/2026.md`.
- **`deleted_at`** — the trash, as a column.

**No `folder` column, deliberately.** The folder is inside `path`, which is the scope prefix
(invariant 4) and the citation identity (invariant 6). A second home for it would disagree the
first time somebody renamed one. The form has folder and name as two boxes because that is how
a person thinks about it, and joins them before sending — the server validates the joined path,
once, in `SafePath`.

`SafePath` split into two doors instead of growing a second rule: it is the **create** rule
(no `qa/`, so an import cannot fabricate an answer a BA vouched for), and `readPath` is the
same structural rules **minus** that refusal, because reading a confirmed answer back — or
fixing a typo in one where it already lives — is not fabrication. A move *into* `qa/` is still
refused. That split pushed `safePath` to gocyclo 17, so `segments()` came out of it; the limit
was not raised.

## The API, and why PUT

`PUT /api/documents/{path…}` is one route for "new" and for "edit", because that is already
what PUT means: this document, at this path. The body carries the attributes and `to`, which is
where it should end up — so a rename is the same sentence with a different address, and the
server moves the chunks with it rather than leaving two documents answering the same question.
A rename is a write-then-remove **in that order**: dropping the old rows first and then failing
to embed would lose the document outright.

`GET /api/documents/{path…}` is **ungated**, and that is invariant 2 read correctly: writes are
gated, reads are open, and this returns text an answer already quotes with a citation pointing
at it. The multipart `POST` stays exactly as it was — bulk drag-and-drop, no attributes,
because one description for eight files is a sentence pretending to describe all of them.

## The screen

`composables/library.js` + `components/LibraryPanel.vue`, wired into `BaScreen.vue`. The list
answers "what do we have?" and the form answers "make this one right", and the form is **below
the list rather than in a dialog** — a BA writes a document while reading what already exists,
and a modal is what takes that away.

It is shown to a locked screen too: reading the library needs no password, so `writes`
(BA_PASS *and* unlocked) is what decides whether any button appears. A read-only instance lists
its documents and offers nothing.

The search field reads path, title, alias, kind and description together, which is the whole
reason `alias` exists. `kinds` and `folders` are `<datalist>` suggestions built from what is
already there, so a BA's own vocabulary converges without a schema for it.

## What this costs, said plainly

**Losing the database loses the corpus.** There is no backup, by decision
(`2026-07-28-no-backup.md`), and the rebuild escape hatch is gone with it: `rm knowledge.db &&
ingest corpus` used to fix any schema mistake for one provider bill, and now there is nothing
to rebuild from. That is why the migration runner shipped first, why it is forward-only, and
why `Remove` is soft. `ingest` still exists as an operator's import client — it reads
`CORPUS_DIR` and calls the same engine, with no privileged path of its own.

## Verified

`make check` green: every package ok, golangci-lint **0 issues** (after `segments()`, not after
a raised limit), deadcode and secrets clean, eslint at `--max-warnings 0`.

Against the built binary with real embeddings, end to end: `PUT` created
`business/pricing/2026.md` with all four attributes; `GET` returned them with the body;
`/api/corpus` listed them; `PUT` with `to` renamed it to `specs/pricing-2026.md` and the old
path left the library; `DELETE` dropped it to 0 docs / 0 chunks while `GET` still returned its
text. Then in a real browser (PinchTab, headless): the panel lists read-only before unlock with
zero buttons, `NEW DOCUMENT` shows all seven fields, filling and saving produced the row with
title · alias · path · description · kind · chunk count, `EDIT` reloaded every field including
the body, and `REMOVE` behind `.perm` emptied the list. Console clean throughout.
