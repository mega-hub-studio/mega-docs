package db

import (
	"fmt"
)

/*
Versioned migrations, forward only.

Why this exists now, when it deliberately did not before: `schema.sql` is
`CREATE TABLE IF NOT EXISTS` throughout, which never delivers a new *column* to a database
that already has the table — the statement finds the table and does nothing. That was
tolerable for exactly as long as `knowledge.db` was **derived**: the upgrade path for any
schema change was `rm knowledge.db && ingest corpus`, and it cost one provider bill.

vNext inverts that. Once the Knowledge DB is the source of truth there is nothing to rebuild
from, so a change that cannot reach an existing database is a change that cannot ship. And
the Knowledge Model vNext asks for needs columns, not just tables: version and status on a
document, a category, a section id on a chunk. New *tables* still arrive fine through
schema.sql; new columns need this.

The order this landed in is the point. This is built **before** the corpus directory stops
being written to, because doing it the other way round removes the way back: the moment the
database is the only copy and its schema cannot migrate, a schema mistake is unrecoverable.

Deliberately small, because a migration runner is infrastructure and infrastructure that is
clever is infrastructure that fails at 3am:

  - Forward only. No down migrations. A down migration is a second code path that runs only
    on the worst day of the year, and it is never the one that was tested.
  - One transaction per migration, so a failure leaves the version where it was rather than
    half-applied.
  - `schema_version` is a table, not a pragma. `user_version` was the obvious choice and is
    wrong here: it holds one integer with no room for *when* or *what*, and the first
    question anyone asks a database that behaves oddly is which migrations it has actually
    seen.
  - Applied after schema.sql, so a fresh database gets the tables and then walks the list
    finding nothing to do. One code path for both, rather than "new database" and "old
    database" diverging.
*/

// migration is one forward step. `id` is the ordering and the identity — never renumber one,
// and never edit the SQL of a migration that has shipped: an instance that already ran it
// will not run it again, so an edit means two databases with the same version and different
// shapes.
type migration struct {
	id  int
	why string // what it is for, read straight out of the table when something looks odd
	sql string
}

// migrations is the whole history, in order.
//
// To add one: append, never insert. The next id is the highest here plus one.
var migrations = []migration{
	{
		id:  1,
		why: "the document body and its attributes live here now: the DB is the source of truth",
		// This is the inversion, in five columns.
		//
		// `body` is the one that changes what this database *is*. Until now a document row
		// was derived — the text lived in CORPUS_DIR and `ingest` rebuilt everything — so
		// losing the database cost one provider bill. The body being here is what makes the
		// WebUI import the only way in: there is no file to reconcile with, and no second
		// spelling of a document to drift.
		//
		// The other four are what a BA needs to find a document again six months later, and
		// they are attributes rather than structure: `folder` is deliberately NOT among them
		// because it is already inside `path`, which is the scope prefix (invariant 4) and
		// the citation identity (invariant 6). A folder column would be the same fact twice,
		// disagreeing the first time somebody renames one.
		//
		// `deleted_at` replaces the .trash/ directory. Remove drops the chunks — the document
		// leaves retrieval immediately, which is the whole request — and keeps the row, so
		// the text is still recoverable by whoever has the database. That is the same deal
		// .trash/ offered (recovery by whoever has the disk), for the price of one column
		// instead of a directory the path rules had to be taught to avoid.
		sql: `
			ALTER TABLE documents ADD COLUMN body        TEXT;
			ALTER TABLE documents ADD COLUMN alias       TEXT;
			ALTER TABLE documents ADD COLUMN kind        TEXT;
			ALTER TABLE documents ADD COLUMN description TEXT;
			ALTER TABLE documents ADD COLUMN deleted_at  TEXT;
		`,
	},
	{
		id:  2,
		why: "documents indexed before the inversion have no body: rebuild it from their chunks",
		// The gap migration 1 left, found on a live instance rather than reasoned about: nine
		// documents that answer questions and whose text the library could neither show nor
		// edit, because they were indexed while the file was the source of truth and the file
		// is gone.
		//
		// `ingest` cannot fix that — it needs the folder those files came from, and on the box
		// where this was found CORPUS_DIR was already empty. The text is not lost, though: the
		// chunks are the document, split. So this puts it back together in `ord` order.
		//
		// It is a reconstruction and says so: a heading line can be missing where an oversized
		// section was split by paragraph, because that piece carries no heading of its own.
		// That is the whole cost, and the alternative is a document nobody can read — so this
		// is the cheaper wrong. Chunk-less documents keep a NULL body rather than an empty
		// string: "we never had the text" and "the text is empty" are different facts, and the
		// second one would make an edit form offer to save nothing over a live document.
		//
		// The document's own H1 is put back rather than left out, and that is not polish: the
		// first chunk's breadcrumb starts with it ("Handoff — Booking List > 1. Purpose"), the
		// first chunk's *content* does not, and without this the first thing a BA saves over a
		// legacy document is one with its title deleted. Only when the text does not already
		// start with a heading, so a document whose first chunk kept its own is left alone.
		sql: `
			UPDATE documents SET body = COALESCE(
				(SELECT CASE
				   WHEN substr(b.text, 1, 1) = '#' THEN b.text
				   WHEN b.h1 = '' THEN b.text
				   ELSE '# ' || b.h1 || char(10) || char(10) || b.text
				 END
				 FROM (
				   SELECT
				     (SELECT group_concat(c.content, char(10) || char(10))
				      FROM (SELECT content FROM chunks
				            WHERE document_id = documents.id ORDER BY ord) c) AS text,
				     COALESCE((SELECT CASE
				         WHEN instr(heading, ' > ') > 0 THEN substr(heading, 1, instr(heading, ' > ') - 1)
				         ELSE heading
				       END
				       FROM chunks WHERE document_id = documents.id ORDER BY ord LIMIT 1), '') AS h1
				 ) b
				 WHERE b.text IS NOT NULL),
				body)
			WHERE body IS NULL;
		`,
	},
	{
		id:  3,
		why: "keyword search could not see what a document is called, only what it says",
		// The half of retrieval that is keyword matching indexed a chunk's content and its
		// heading, and nothing else — so the four things a person files a document under were
		// invisible to it. A document called `decision-4-auth.md`, titled "decision 4 auth",
		// filed under kind `decision`, whose text says "Phiên đăng nhập lưu ở cookie HttpOnly"
		// and never uses the word "decision", could not be found by that word at all.
		//
		// Measured before this: on a 52-document corpus, "có bao nhiêu decision" came back with
		// a table of unrelated notes and named none of the seven decisions — the notes won
		// because their *bodies* happened to contain "quyết định". The count is answered from
		// the rows now (see rag/smalltalk.go), but every other question about a document by its
		// name was answered the same wrong way and had no such escape.
		//
		// The fix is in the index rather than in ranking, and that is the whole point: with the
		// identity in a column, BM25 scores it like any other text and RRF fuses two legs
		// exactly as it did. No third candidate source, no hand-tuned boost, and
		// `toFTSQuery` needs no change because it never scoped a term to a column.
		//
		// Cost is a rebuild of the keyword index, which is pure SQL — no embedding is touched,
		// so this costs nothing with a provider and needs no re-ingest. Triggers go first so
		// the backfill does not fire the old ones once per chunk.
		//
		// `ident` repeats per chunk, which is the price of an external-content FTS: the
		// alternative is a standalone table holding a second copy of every chunk's *text*.
		// A path and three short attributes against six hundred characters of content.
		sql: `
			DROP TRIGGER IF EXISTS chunks_ai;
			DROP TRIGGER IF EXISTS chunks_ad;
			DROP TRIGGER IF EXISTS chunks_au;

			ALTER TABLE chunks ADD COLUMN ident TEXT;
			UPDATE chunks SET ident = (
			  SELECT d.path || ' ' || COALESCE(d.title,'') || ' ' ||
			         COALESCE(d.alias,'') || ' ' || COALESCE(d.kind,'')
			  FROM documents d WHERE d.id = chunks.document_id);

			DROP TABLE IF EXISTS fts_chunks;
			CREATE VIRTUAL TABLE fts_chunks USING fts5(
			  content,
			  heading,
			  ident,
			  content='chunks',
			  content_rowid='id',
			  tokenize='unicode61 remove_diacritics 2'
			);
			CREATE TRIGGER chunks_ai AFTER INSERT ON chunks BEGIN
			  INSERT INTO fts_chunks(rowid, content, heading, ident)
			  VALUES (new.id, new.content, new.heading, new.ident);
			END;
			CREATE TRIGGER chunks_ad AFTER DELETE ON chunks BEGIN
			  INSERT INTO fts_chunks(fts_chunks, rowid, content, heading, ident)
			  VALUES('delete', old.id, old.content, old.heading, old.ident);
			END;
			CREATE TRIGGER chunks_au AFTER UPDATE ON chunks BEGIN
			  INSERT INTO fts_chunks(fts_chunks, rowid, content, heading, ident)
			  VALUES('delete', old.id, old.content, old.heading, old.ident);
			  INSERT INTO fts_chunks(rowid, content, heading, ident)
			  VALUES (new.id, new.content, new.heading, new.ident);
			END;
			INSERT INTO fts_chunks(fts_chunks) VALUES('rebuild');
		`,
	},
}

// migrateVersioned applies whatever has not run yet, and reports how many it applied.
func (s *Store) migrateVersioned() (int, error) {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		id         INTEGER PRIMARY KEY,
		why        TEXT NOT NULL DEFAULT '',
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return 0, fmt.Errorf("schema_version: %w", err)
	}

	// Read the applied set in its own function so the rows can be closed with defer: the
	// linter is right that a hand-rolled Close on every error path is where a leak hides.
	applied, err := s.appliedMigrations()
	if err != nil {
		return 0, err
	}

	n := 0
	for _, m := range migrations {
		if applied[m.id] {
			continue
		}
		// One transaction per migration: the version row and the change it describes commit
		// together, so an interrupted start can never leave a database claiming a version
		// whose SQL only half ran.
		tx, err := s.db.Begin()
		if err != nil {
			return n, err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return n, fmt.Errorf("migration %d (%s): %w", m.id, m.why, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version(id, why) VALUES(?,?)`, m.id, m.why); err != nil {
			_ = tx.Rollback()
			return n, fmt.Errorf("recording migration %d: %w", m.id, err)
		}
		if err := tx.Commit(); err != nil {
			return n, fmt.Errorf("committing migration %d: %w", m.id, err)
		}
		n++
	}
	return n, nil
}

// appliedMigrations is the set of ids this database has already run.
func (s *Store) appliedMigrations() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT id FROM schema_version`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	applied := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		applied[id] = true
	}
	return applied, rows.Err()
}

// There was a `hasColumn` helper here, written for the "add a column" migrations that do not
// exist yet. `make dead` reported it unreachable from any binary — only its own test called
// it — which is critical rule 17 arriving on schedule: the first migration that genuinely
// needs to ask SQLite whether a column is already there can write the pragma query at the
// point it asks, with the reason it asks in that migration's `why`.
