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

	docID, err := s.UpsertDocument(Doc{Path: "docs/auth.md", Title: "auth"})
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
	hits, err := s.Search(vec(0.9, 0.1, 0, 0), "how do I get a JWT token", 3, "", false)
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

	authID, err := s.UpsertDocument(Doc{Path: "docs/auth.md", Title: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	rateID, err := s.UpsertDocument(Doc{Path: "docs/rate.md", Title: "rate limits"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
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

// TestScopedSearchRanksWithinTheScope is the check that the scope is a *pre*-filter on
// both retrievers, not a filter over their results.
//
// The corpus is stacked against it: twenty chunks outside the scope are the nearest
// vectors *and* the strongest keyword matches, and only five far-away chunks are
// inside it. A post-filter would take the global top-k first and return nothing (or a
// handful); a pre-filter returns k hits, all from the scope. sqlite-vec does the
// former as of v0.1.6 — if a later version regresses, this test says so instead of
// scoped answers quietly going thin.
func TestScopedSearchRanksWithinTheScope(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "scope.db"), 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	loud, err := s.UpsertDocument(Doc{Path: "support/faq.md", Title: "faq"})
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := s.UpsertDocument(Doc{Path: "booking/calendar/rules.md", Title: "rules"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		if err := s.InsertChunk(loud, "FAQ", "refund window and refund policy", i, vec(1, float32(i)*0.001, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 5 {
		if err := s.InsertChunk(quiet, "Rules", "a refund inside the booking calendar", i, vec(0, 0, 1, float32(i)*0.001)); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range []struct {
		name, scope string
		wantDoc     string
		wantHits    int
	}{
		{"the whole corpus still ranks globally", "", "support/faq.md", 3},
		{"a folder scope", "booking/calendar", "booking/calendar/rules.md", 3},
		{"a folder above it", "booking", "booking/calendar/rules.md", 3},
		{"one document", "booking/calendar/rules.md", "booking/calendar/rules.md", 3},
		{"a scope that matches nothing retrieves nothing", "engineering", "", 0},
		// "booking" must not also match a sibling that merely starts with it, which is
		// what a bare LIKE 'booking%' would do.
		{"a prefix that is not a path segment", "book", "", 0},
	} {
		hits, err := s.Search(vec(1, 0, 0, 0), "refund policy", 3, c.scope, false)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(hits) != c.wantHits {
			t.Errorf("%s: %d hits; want %d", c.name, len(hits), c.wantHits)
		}
		for _, h := range hits {
			if h.DocPath != c.wantDoc {
				t.Errorf("%s: hit from %s; want only %s", c.name, h.DocPath, c.wantDoc)
			}
		}
	}
}

// TestOneDocumentCannotFillTheAnswer is the breadth half of retrieval, and it is a real
// failure rather than a preference: the corpus here has one document whose every chunk
// out-ranks everything else, so without the cap a ten-section answer is ten sections of one
// file — thorough-looking and narrow. The cap keeps that document's best three and lets the
// rest fall to whoever is next.
//
// TestSearchStitchesOrdAdjacentNeighbours rides along, because the same fixture answers it:
// SplitMarkdown cuts at a size rather than at a thought, so a chunk that starts mid-argument
// is what makes an answer read as if it skipped a step. Asking with withNeighbours=true must
// grow each hit's Content without changing how many hits there are or which they are.
func TestOneDocumentCannotFillTheAnswer(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cap.db"), 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// The loud document ranks first on both legs for every chunk; the quiet ones are further
	// away but still match the keyword, so only the cap can get them into the answer.
	loud, err := s.UpsertDocument(Doc{Path: "specs/loud.md", Title: "loud"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		if err := s.InsertChunk(loud, "Loud", "refund policy paragraph", i, vec(1, float32(i)*0.001, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"specs/quiet-a.md", "specs/quiet-b.md"} {
		id, err := s.UpsertDocument(Doc{Path: path, Title: path})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.InsertChunk(id, "Quiet", "refund policy elsewhere", 0, vec(0, 1, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := s.Search(vec(1, 0, 0, 0), "refund policy", 10, "", false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	per := map[string]int{}
	for _, h := range hits {
		per[h.DocPath]++
	}
	if per["specs/loud.md"] > maxPerDoc {
		t.Errorf("one document contributed %d of %d hits; the cap is %d",
			per["specs/loud.md"], len(hits), maxPerDoc)
	}
	if len(per) < 2 {
		t.Errorf("every hit came from %v — the cap exists so the freed slots go somewhere", per)
	}

	stitched, err := s.Search(vec(1, 0, 0, 0), "refund policy", 10, "", true)
	if err != nil {
		t.Fatalf("stitched search: %v", err)
	}
	if len(stitched) != len(hits) {
		t.Fatalf("stitching changed the hit count: %d, was %d — a citation numbers one hit",
			len(stitched), len(hits))
	}
	for i, h := range stitched {
		if h.ChunkID != hits[i].ChunkID {
			t.Fatalf("hit %d is chunk %d, was %d — stitching may not reorder", i, h.ChunkID, hits[i].ChunkID)
		}
	}
	// The loud document's kept chunks have neighbours on at least one side, so at least one
	// hit must have read more than it did unstitched. A run where none did means the join
	// found nothing and the whole step is silently a no-op.
	grew := 0
	for i, h := range stitched {
		if len(h.Content) > len(hits[i].Content) {
			grew++
		}
	}
	if grew == 0 {
		t.Error("no hit grew: neighbour stitching read nothing, and an answer still sees the cut")
	}
}

// A document path containing a LIKE metacharacter is matched as itself. Without the
// escape, scoping to "q_1" would also answer from "qa1", citing a folder nobody asked
// for.
func TestScopeTreatsWildcardsAsCharacters(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "wild.db"), 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	underscore, _ := s.UpsertDocument(Doc{Path: "q_1/spec.md", Title: "underscore"})
	other, _ := s.UpsertDocument(Doc{Path: "qa1/spec.md", Title: "other"})
	if err := s.InsertChunk(underscore, "H", "the escaped one", 0, vec(1, 0, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertChunk(other, "H", "the escaped one", 0, vec(1, 0, 0, 0)); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search(vec(1, 0, 0, 0), "escaped", 5, "q_1", false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].DocPath != "q_1/spec.md" {
		t.Errorf("hits = %+v; want only q_1/spec.md", hits)
	}
}

// What a document is *called* has to be findable, not only what it says.
//
// The keyword half of retrieval indexed content and heading and nothing else, so the four
// things a person files a document under were invisible to it. Measured on a 52-document
// corpus before this: "có bao nhiêu decision" returned a table of unrelated notes and named
// none of the seven decisions — the notes won because their bodies happened to contain
// "quyết định", while every decision carried the word only in its name.
//
// The document here is deliberately hostile to the old index: its text is about cookies and
// never uses "decision" or "auth", so a hit can only come from the identity column.
func TestADocumentIsFoundByWhatItIsCalled(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ident.db"), 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	wanted, err := s.UpsertDocument(Doc{
		Path: "decision-4-auth.md", Title: "decision 4 auth",
		Alias: "đăng nhập", Kind: "decision",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.InsertChunk(wanted, "Xác thực",
		"Phiên đăng nhập lưu ở cookie HttpOnly, không dùng token trong localStorage.", 0,
		vec(0, 0, 0, 1)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// A decoy whose *text* is what a naive keyword search would rank first.
	noise, err := s.UpsertDocument(Doc{Path: "notes/topic.md", Title: "note"})
	if err != nil {
		t.Fatalf("upsert noise: %v", err)
	}
	if err := s.InsertChunk(noise, "Ghi chú",
		"Một quyết định kỹ thuật của hệ thống, ghi lại lý do và ràng buộc.", 0,
		vec(1, 0, 0, 0)); err != nil {
		t.Fatalf("insert noise: %v", err)
	}

	// Each of the four identity fields, on a query whose vector points at the decoy — so
	// anything that comes back for the right document came back on the keyword leg.
	for _, q := range []string{"decision", "decision-4-auth.md", "đăng nhập", "auth"} {
		hits, err := s.Search(vec(1, 0, 0, 0), q, 5, "", false)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		found := false
		for _, h := range hits {
			if h.DocPath == "decision-4-auth.md" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q found nothing called that — identity is not in the index", q)
		}
	}
}
