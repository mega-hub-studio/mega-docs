-- Source documents (one row per file, versioned)
CREATE TABLE IF NOT EXISTS documents (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  path       TEXT    NOT NULL UNIQUE,
  title      TEXT,
  version    INTEGER NOT NULL DEFAULT 1,
  updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Retrievable units.
-- status/version are the Phase-1 hooks (BA approval + versioning). Cost ~0 now.
CREATE TABLE IF NOT EXISTS chunks (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  heading     TEXT,                              -- breadcrumb, e.g. "Retrieval Flow > Rerank"
  content     TEXT    NOT NULL,
  ord         INTEGER NOT NULL DEFAULT 0,
  status      TEXT    NOT NULL DEFAULT 'draft',  -- draft | approved
  version     INTEGER NOT NULL DEFAULT 1
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
