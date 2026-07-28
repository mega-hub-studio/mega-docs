package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// The whole reason this runner exists, proven the only way that counts: against a database
// created by an OLDER schema.
//
// A test that opens a fresh store and finds its columns proves nothing — schema.sql created
// them. This creates `documents` *without* a column, then runs a migration that adds it, and
// checks it arrived. That is precisely what `CREATE TABLE IF NOT EXISTS` cannot do, and what
// makes a DB-as-source-of-truth safe to ship.
func TestAColumnReachesAnExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// An "old" database: the table exists, without the column a later version wants.
	old, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT NOT NULL UNIQUE, title TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO documents(path,title) VALUES('booking/a.md','a')`); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	// A migration that adds it, installed for the duration of this test.
	defer func(saved []migration) { migrations = saved }(migrations)
	migrations = []migration{{
		id:  9001,
		why: "test: version on a document",
		sql: `ALTER TABLE documents ADD COLUMN doc_version INTEGER NOT NULL DEFAULT 1`,
	}}

	s, err := Open(path, 4)
	if err != nil {
		t.Fatalf("opening a database created by an older schema: %v", err)
	}
	defer func() { _ = s.Close() }()

	has, err := s.hasColumn("documents", "doc_version")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("the column did not reach the existing database — the runner is not doing its job")
	}

	// The existing row survived, which ALTER TABLE ... ADD COLUMN guarantees and a
	// drop-and-recreate would not. Worth asserting: the day someone writes a migration as
	// CREATE-new-then-copy, this is what catches the row it forgot.
	var kept int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE path='booking/a.md'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("the migration lost the row it was supposed to migrate (%d found)", kept)
	}

	// And it is recorded, with its reason — the first question asked of a database behaving
	// oddly is which migrations it has actually seen.
	var why string
	if err := s.db.QueryRow(`SELECT why FROM schema_version WHERE id=9001`).Scan(&why); err != nil {
		t.Fatalf("the migration was applied but not recorded: %v", err)
	}
	if why == "" {
		t.Error("schema_version recorded no reason")
	}
}

// Forward only, and idempotent: a second start must not re-run what already ran. Without
// this, every restart would re-apply every ALTER and fail on "duplicate column name".
func TestAMigrationRunsExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.db")

	defer func(saved []migration) { migrations = saved }(migrations)
	migrations = []migration{{
		id:  9002,
		why: "test: runs once",
		sql: `ALTER TABLE documents ADD COLUMN once_only TEXT NOT NULL DEFAULT ''`,
	}}

	first, err := Open(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	n, err := first.migrateVersioned()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a second pass applied %d migrations; Open already ran them", n)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// A restart is the real case: the process comes back and must not fall over.
	second, err := Open(path, 4)
	if err != nil {
		t.Fatalf("reopening re-applied a migration: %v", err)
	}
	defer func() { _ = second.Close() }()
}

// A failing migration must leave the version alone, so the next start retries it rather than
// believing a half-applied change succeeded.
func TestAFailedMigrationIsNotRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.db")

	defer func(saved []migration) { migrations = saved }(migrations)
	migrations = []migration{{id: 9003, why: "test: broken", sql: `ALTER TABLE nope ADD COLUMN x TEXT`}}

	if _, err := Open(path, 4); err == nil {
		t.Fatal("a broken migration was accepted; the engine started on an unknown schema")
	}

	// And nothing claims to have applied it.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM schema_version WHERE id=9003`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("a failed migration was recorded as applied — the next start would skip it")
	}
}
