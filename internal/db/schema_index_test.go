package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// The index is load-bearing for both things the product is judged on: a scoped search that
// reads only an index instead of every chunk's content, and a re-ingest that deletes one
// document's chunks without walking the rest.
//
// Asserted against a real opened store rather than by grepping schema.sql, because the
// question is whether the statement *arrived* — and the reason this repo can add an index
// at all is that CREATE INDEX IF NOT EXISTS applies to a database that already exists,
// unlike a column. A test that read the file would pass on a database that never got it.
func TestChunksAreIndexedByDocument(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "index.db"), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var sql string
	err = s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='index' AND name='chunks_document_id'`,
	).Scan(&sql)
	if err != nil {
		t.Fatalf("chunks_document_id is missing: %v — a scoped search then reads every "+
			"chunk's content to answer which document it is in", err)
	}
	if !strings.Contains(sql, "document_id") {
		t.Errorf("chunks_document_id does not index document_id: %s", sql)
	}

	// And the planner must actually use it: an index nothing chooses is a slower write for
	// no read. This is the scope filter's own subquery, verbatim from scopeFilter.
	rows, err := s.db.Query(
		`EXPLAIN QUERY PLAN
		 SELECT c.id FROM chunks c JOIN documents d ON d.id = c.document_id
		 WHERE d.path = ? OR d.path LIKE ? ESCAPE '\'`, "booking", "booking/%")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var a, b, c int
		var detail string
		if err := rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "chunks_document_id") {
		t.Errorf("the scope filter does not use chunks_document_id. Plan was:\n%s", plan.String())
	}
}
