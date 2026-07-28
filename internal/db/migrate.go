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
// It is empty on purpose. Every column the current schema needs is already in schema.sql,
// and adding a no-op migration to prove the runner works would be exactly the kind of
// ceremony this repo argues against — the runner is proven by its test, which creates a
// database from an older schema and watches a column arrive.
//
// To add one: append, never insert. The next id is the highest here plus one.
var migrations = []migration{}

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

// hasColumn reports whether a table already has a column. Exported to the package because
// every "add a column" migration wants it: SQLite has no ADD COLUMN IF NOT EXISTS, and a
// migration that has already run by other means (a hand-patched instance, a restored backup
// from a newer version) must not fail the start-up of the whole engine.
func (s *Store) hasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf(`SELECT 1 FROM pragma_table_info(%q) WHERE name = ?`, table), column)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	if err := rows.Err(); err != nil {
		return false, err
	}
	return found, nil
}
