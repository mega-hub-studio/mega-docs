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
