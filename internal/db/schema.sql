-- Source documents (one row per file). updated_at is bumped on every re-ingest, and
-- is part of the answer cache's corpus signature — so re-indexing invalidates it.
CREATE TABLE IF NOT EXISTS documents (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  path       TEXT    NOT NULL UNIQUE,
  title      TEXT,
  updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Retrievable units.
-- status is real: a BA confirming an answer marks that document's chunks 'approved',
-- and Search() boosts them. It is the only part of the corpus a person vouched for.
CREATE TABLE IF NOT EXISTS chunks (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  heading     TEXT,                              -- breadcrumb, e.g. "Retrieval Flow > Rerank"
  content     TEXT    NOT NULL,
  ord         INTEGER NOT NULL DEFAULT 0,
  status      TEXT    NOT NULL DEFAULT 'draft'   -- draft | approved
);

-- Every query that touches a chunk by its document goes through this: the scope filter
-- (both retrievers), the Search join, the per-document DELETE on re-ingest, the corpus
-- stats LEFT JOIN, and the confirm that marks a document's chunks approved.
--
-- Without it, scoping a search is `SCAN c` — every chunk row read whole, including the
-- `content` TEXT, to answer a question about which document it belongs to. With it the
-- planner reports `SCAN c USING COVERING INDEX chunks_document_id`: the same walk over the
-- index alone, so a corpus of 100k chunks reads a few hundred KB instead of every byte of
-- every chunk. Measured with EXPLAIN QUERY PLAN, before and after, on a real database.
--
-- An index is also the one schema change this repo can make for free. "The schema has no
-- migrations" is about *columns*: CREATE TABLE IF NOT EXISTS does nothing to a table that
-- already exists, so a new column never arrives. CREATE INDEX IF NOT EXISTS does arrive —
-- an index is derived from rows that are already there. So this needs no re-ingest and no
-- provider bill, on a running deployment, which is exactly why it belongs here rather than
-- in a note about a future rebuild.
CREATE INDEX IF NOT EXISTS chunks_document_id ON chunks(document_id);

-- QA tickets: the BA ⇄ DEV loop. A DEV files the question the documents could
-- not answer; a BA answers it and confirms that answer into the corpus, where the
-- next DEV retrieves it with a citation.
--
-- status is the whole state machine, and every value is reachable by one action:
--   open      DEV filed a gap                     (POST /api/tickets)
--   answered  BA saved a draft, not yet indexed   (…/draft — survives a phone)
--   confirmed indexed + approved, retrievable     (…/confirm)
--   rejected  not a documentation gap             (…/reject)
CREATE TABLE IF NOT EXISTS tickets (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  question   TEXT    NOT NULL,
  q_norm     TEXT    NOT NULL,              -- dedupe key; see db.normalise
  miss       TEXT    NOT NULL DEFAULT '',   -- what the engine answered instead
  status     TEXT    NOT NULL DEFAULT 'open',
  answer     TEXT    NOT NULL DEFAULT '',
  note       TEXT    NOT NULL DEFAULT '',   -- why it was dismissed
  doc_path   TEXT    NOT NULL DEFAULT '',   -- the document a confirm created
  asked_at   TEXT    NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- One open ticket per question, not one per ask: without this, three devs hitting
-- the same gap on the same morning give the BA three identical tickets.
CREATE UNIQUE INDEX IF NOT EXISTS tickets_open_q
  ON tickets(q_norm) WHERE status IN ('open','answered');

-- Answer cache. A repeated question is answered from here — no embedding call, no
-- completion, no cost — and every row is also one line of the History panel.
--
-- corpus_sig is the invalidation rule: it changes whenever a document is ingested
-- or a ticket is confirmed, so an answer can never outlive the documents it cited.
CREATE TABLE IF NOT EXISTS answers (
  q_norm     TEXT PRIMARY KEY,
  question   TEXT    NOT NULL,
  answer     TEXT    NOT NULL,
  citations  TEXT    NOT NULL DEFAULT '[]',
  corpus_sig TEXT    NOT NULL,
  hits       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL DEFAULT (datetime('now')),
  used_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- BM25 keyword index kept in sync with chunks via triggers.
CREATE VIRTUAL TABLE IF NOT EXISTS fts_chunks USING fts5(
  content,
  heading,
  content='chunks',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
  INSERT INTO fts_chunks(rowid, content, heading) VALUES (new.id, new.content, new.heading);
END;
CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
  INSERT INTO fts_chunks(fts_chunks, rowid, content, heading) VALUES('delete', old.id, old.content, old.heading);
END;
CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
  INSERT INTO fts_chunks(fts_chunks, rowid, content, heading) VALUES('delete', old.id, old.content, old.heading);
  INSERT INTO fts_chunks(rowid, content, heading) VALUES (new.id, new.content, new.heading);
END;
