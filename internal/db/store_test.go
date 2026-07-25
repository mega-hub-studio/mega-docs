package db

import (
	"path/filepath"
	"testing"
)

func vec(a, b, c, d float32) []float32 { return []float32{a, b, c, d} }

// TestHybridSearch verifies the vec0 table, FTS5 triggers, and RRF fusion
// work together against a real (temp) SQLite file. No network / API needed.
func TestHybridSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path, 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	docID, err := s.UpsertDocument("docs/auth.md", "auth")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	chunks := []struct {
		text string
		emb  []float32
	}{
		{"The login endpoint returns a JWT token on success.", vec(1, 0, 0, 0)},
		{"Rate limiting is enforced per API key.", vec(0, 1, 0, 0)},
		{"Database migrations run automatically at startup.", vec(0, 0, 1, 0)},
	}
	for i, c := range chunks {
		if err := s.InsertChunk(docID, "Auth", c.text, i, c.emb); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	// Query semantically close to chunk 0, and keyword "JWT" also in chunk 0.
	hits, err := s.Search(vec(0.9, 0.1, 0, 0), "how do I get a JWT token", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits, got none")
	}
	if hits[0].ChunkID == 0 || hits[0].Content == "" {
		t.Fatalf("bad top hit: %+v", hits[0])
	}
	t.Logf("top hit: %q (score %.4f)", hits[0].Content, hits[0].Score)
}

// TestCorpus checks the counts and the per-document rollup against a real DB —
// the numbers the UI shows instead of leaving an empty index looking broken.
func TestCorpus(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "corpus.db"), 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// An untouched index must report zeroes, not an error and not nil.
	empty, err := s.Corpus(0)
	if err != nil {
		t.Fatalf("empty corpus: %v", err)
	}
	if empty.Docs != 0 || empty.Chunks != 0 || empty.Documents == nil {
		t.Fatalf("empty corpus = %+v; want zeroes and a non-nil slice", empty)
	}

	authID, err := s.UpsertDocument("docs/auth.md", "auth")
	if err != nil {
		t.Fatal(err)
	}
	rateID, err := s.UpsertDocument("docs/rate.md", "rate limits")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.InsertChunk(authID, "H", "auth body", i, vec(1, 0, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.InsertChunk(rateID, "H", "rate body", 0, vec(0, 1, 0, 0)); err != nil {
		t.Fatal(err)
	}

	c, err := s.Corpus(0)
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if c.Docs != 2 || c.Chunks != 4 {
		t.Errorf("totals = %d docs / %d chunks; want 2 / 4", c.Docs, c.Chunks)
	}
	if c.Approved != 0 {
		t.Errorf("approved = %d; chunks default to draft", c.Approved)
	}
	if len(c.Documents) != 2 {
		t.Fatalf("documents = %d; want 2", len(c.Documents))
	}
	per := map[string]int{}
	for _, d := range c.Documents {
		per[d.Path] = d.Chunks
		if d.UpdatedAt == "" {
			t.Errorf("%s has no updated_at", d.Path)
		}
	}
	if per["docs/auth.md"] != 3 || per["docs/rate.md"] != 1 {
		t.Errorf("per-document chunk counts = %v; want auth 3, rate 1", per)
	}

	// limit is a cap, not a suggestion — the payload has to stay phone-sized.
	if one, err := s.Corpus(1); err != nil || len(one.Documents) != 1 {
		t.Errorf("Corpus(1) = %d docs, %v; want 1 and no error", len(one.Documents), err)
	}
}
