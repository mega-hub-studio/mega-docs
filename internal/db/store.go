// Package db is the only place that knows SQLite: sqlite-vec for embeddings, FTS5 for
// keywords, hybrid retrieval fused with RRF, the QA ticket state machine, and the answer
// cache. Nothing above it imports a driver, and it imports nothing of the domain.
package db

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	// Registered for its side effect: the "sqlite3" driver name this package opens.
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

// Store is an open database. One value per process; the zero value is not usable — call
// Open, which also applies the schema.
type Store struct {
	db  *sql.DB
	dim int
	// Keep caps how many cached answers survive a prune. Zero means the package default —
	// exported and settable rather than a constructor argument, because it is the one field
	// here an operator tunes and Open() already takes the two that decide whether the file
	// can be opened at all.
	Keep int
}

// statusApproved is the chunk status a BA confirm produces. The SQL statements spell it
// out because there it is part of a query; in Go it is compared against, so it is a
// constant — a typo in a string comparison is a boost that silently never applies.
const statusApproved = "approved"

// Hit is one retrieved chunk with provenance for citation. DocumentID and Ord are not
// provenance a reader ever sees — they are what stitchNeighbours needs to find the chunks
// either side of this one, which is the only reason they leave this package.
type Hit struct {
	ChunkID    int
	DocumentID int
	Ord        int
	DocPath    string
	Heading    string
	Content    string
	Status     string
	Score      float64
}

// CandidatePool is how many candidates each retriever offers before fusion, and therefore
// the widest an answer can ever be. Exported because the engine asks for exactly this many
// when it is filling a context budget rather than a fixed count: two numbers for "as wide
// as retrieval goes" would disagree the first time one of them moved.
const CandidatePool = 40

// maxPerDoc caps how many chunks one document may contribute to an answer.
//
// Without it the whole budget can go to a single file — six near-identical sections of one
// spec, which reads as a thorough answer and is a narrow one. The cap costs nothing: the
// slots freed go to the next-best chunk of some other document, so the answer is the same
// size and covers more of the corpus.
const maxPerDoc = 3

// Open opens (or creates) the SQLite DB and runs migrations.
// dim = embedding dimension (must match your embedding model).
func Open(path string, dim int) (*Store, error) {
	sqlite_vec.Auto() // register sqlite-vec as an auto-loaded extension
	sdb, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	s := &Store{db: sdb, dim: dim}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	// vec table dimension is dynamic, so create it here.
	q := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
			chunk_id INTEGER PRIMARY KEY,
			embedding FLOAT[%d]
		)`, s.dim)
	if _, err := s.db.Exec(q); err != nil {
		return err
	}
	// After schema.sql, so a fresh database gets its tables and then walks the migration
	// list finding nothing to do — one code path instead of "new" and "existing" diverging.
	// This is what lets a *column* reach a database that already exists, which
	// CREATE TABLE IF NOT EXISTS never could. See migrate.go for why that matters now.
	_, err := s.migrateVersioned()
	return err
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// Doc is one document as it is written: its identity, its text, and the attributes a BA
// files it under. One struct rather than six positional arguments, because the call site is
// where a swapped alias and description would go unnoticed.
//
// Path carries the folder — it is the scope prefix and the citation identity, so there is
// deliberately no separate folder field. Body is the document itself: it lives here, and
// nowhere else.
type Doc struct {
	Path        string
	Title       string
	Alias       string
	Kind        string
	Description string
	Body        string
}

// UpsertDocument replaces a document and all its chunks (idempotent re-ingest).
//
// Writing the same path again resurrects a removed document rather than leaving a row that
// is both present and deleted: `deleted_at` is cleared, which is what "import it again"
// means to whoever does it.
func (s *Store) UpsertDocument(d Doc) (int64, error) {
	path := d.Path
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Delete old chunks + vectors, bump version.
	var oldID sql.NullInt64
	_ = tx.QueryRow(`SELECT id FROM documents WHERE path=?`, path).Scan(&oldID)
	if oldID.Valid {
		_, _ = tx.Exec(`DELETE FROM vec_chunks WHERE chunk_id IN (SELECT id FROM chunks WHERE document_id=?)`, oldID.Int64)
		_, _ = tx.Exec(`DELETE FROM chunks WHERE document_id=?`, oldID.Int64)
	}
	if _, err := tx.Exec(
		`INSERT INTO documents(path,title,alias,kind,description,body,updated_at,deleted_at)
		 VALUES(?,?,?,?,?,?,datetime('now'),NULL)
		 ON CONFLICT(path) DO UPDATE SET title=excluded.title,
		   alias=excluded.alias, kind=excluded.kind, description=excluded.description,
		   body=excluded.body, updated_at=datetime('now'), deleted_at=NULL`,
		path, d.Title, d.Alias, d.Kind, d.Description, d.Body); err != nil {
		return 0, err
	}
	var docID int64
	if err := tx.QueryRow(`SELECT id FROM documents WHERE path=?`, path).Scan(&docID); err != nil {
		return 0, err
	}
	return docID, tx.Commit()
}

// InsertChunk stores one chunk plus its embedding.
func (s *Store) InsertChunk(docID int64, heading, content string, ord int, emb []float32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// `ident` is read off the document rather than passed in, because the row is already
	// written by the time any chunk is: `ingest` upserts the document and then inserts its
	// chunks. A parameter would be a second copy of what the row already says, and the first
	// caller to forget it would leave a document findable by its text and not by its name.
	res, err := tx.Exec(
		`INSERT INTO chunks(document_id,heading,content,ord,ident)
		 VALUES(?,?,?,?, (SELECT path || ' ' || COALESCE(title,'') || ' ' ||
		                         COALESCE(alias,'') || ' ' || COALESCE(kind,'')
		                  FROM documents WHERE id=?))`,
		docID, heading, content, ord, docID)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()

	blob, err := sqlite_vec.SerializeFloat32(emb)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO vec_chunks(chunk_id, embedding) VALUES(?, ?)`, id, blob); err != nil {
		return err
	}
	return tx.Commit()
}

// Search runs hybrid retrieval: vector KNN + BM25, fused with Reciprocal Rank Fusion.
// Approved chunks — the answers a BA confirmed — get a small boost, so a human-vouched
// section wins a tie against one that merely mentions the same words.
//
// scope narrows retrieval to one part of the corpus — a document path or the folder
// above it; "" is everything. Both retrievers are filtered *before* they rank, never
// after fusion: dropping out-of-scope rows afterwards leaves fewer than k results and
// degrades the answer without saying so.
//
// withNeighbours grows each hit to the chunks either side of it. The caller decides,
// because the thing that makes it affordable — a model window big enough to read them —
// is a fact about the model, and this package does not know there is one.
func (s *Store) Search(qEmb []float32, qText string, k int, scope string, withNeighbours bool) ([]Hit, error) {
	vecOrder, err := s.vectorCandidates(qEmb, scope, CandidatePool)
	if err != nil {
		return nil, err
	}
	// The keyword leg is best-effort by design: a query FTS5 refuses must cost the
	// answer some recall, never the whole answer, because the vector leg can still
	// carry it. So this one returns no error to check.
	ftsOrder := s.keywordCandidates(qText, scope, CandidatePool)

	score := fuse(vecOrder, ftsOrder)
	if len(score) == 0 {
		return nil, nil
	}
	hits, err := s.hits(score, k)
	if err != nil || !withNeighbours {
		return hits, err
	}
	return s.stitchNeighbours(hits)
}

// vectorCandidates returns chunk ids by embedding distance, nearest first.
func (s *Store) vectorCandidates(qEmb []float32, scope string, pool int) ([]int, error) {
	blob, err := sqlite_vec.SerializeFloat32(qEmb)
	if err != nil {
		return nil, err
	}
	within, args := scopeFilter("chunk_id", scope)
	//nolint:gosec // G202: `within` is a constant SQL fragment from scopeFilter with ?
	// placeholders; every value, including the scope, is bound as a parameter.
	rows, err := s.db.Query(
		`SELECT chunk_id FROM vec_chunks WHERE embedding MATCH ? AND k = ?`+within+
			` ORDER BY distance`,
		append([]any{blob, pool}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("vec search: %w", err)
	}
	defer rows.Close()

	var order []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		order = append(order, id)
	}
	// A failed iteration returns the rows it managed to read, which is indistinguishable
	// from a corpus that simply had fewer matches — so it has to be an error.
	return order, rows.Err()
}

// keywordCandidates returns chunk ids by BM25 rank, best first. Best-effort: an empty
// result and a refused query are the same answer here, because the vector leg is still
// in play. See the call in Search.
func (s *Store) keywordCandidates(qText, scope string, pool int) []int {
	match := toFTSQuery(qText)
	if match == "" {
		return nil
	}
	within, args := scopeFilter("rowid", scope)
	//nolint:gosec // G202: same as vectorCandidates — fixed fragment, bound values.
	rows, err := s.db.Query(
		`SELECT rowid FROM fts_chunks WHERE fts_chunks MATCH ?`+within+
			` ORDER BY rank LIMIT ?`,
		append(append([]any{match}, args...), pool)...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var order []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			order = append(order, id)
		}
	}
	// Best-effort, deliberately: rows.Err() is read so a truncated iteration is not
	// mistaken for the end of the results, and then discarded, because the caller can
	// still answer from the vector leg. This is the one place in the store that does
	// that, and it is why keywordCandidates returns no error.
	_ = rows.Err()
	return order
}

// fuse combines two ranked id lists with Reciprocal Rank Fusion: a chunk both
// retrievers found beats one that only ranks highly in either.
func fuse(rankings ...[]int) map[int]float64 {
	const rrfK = 60.0

	score := map[int]float64{}
	for _, order := range rankings {
		for rank, id := range order {
			score[id] += 1.0 / (rrfK + float64(rank))
		}
	}
	return score
}

// hits loads the bodies and provenance for the fused ids, best score first, capped at k.
// Approved chunks — the answers a BA confirmed — get a small boost, so a human-vouched
// section wins a tie against one that merely mentions the same words.
func (s *Store) hits(score map[int]float64, k int) ([]Hit, error) {
	args := make([]any, 0, len(score))
	for id := range score {
		args = append(args, id)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(args)), ",")
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT c.id, c.document_id, c.ord, d.path, c.heading, c.content, c.status
		 FROM chunks c JOIN documents d ON d.id = c.document_id
		 WHERE c.id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(
			&h.ChunkID, &h.DocumentID, &h.Ord, &h.DocPath, &h.Heading, &h.Content, &h.Status,
		); err != nil {
			return nil, err
		}
		h.Score = score[h.ChunkID]
		if h.Status == statusApproved {
			h.Score *= 1.2 // a person vouched for this one
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	hits = capPerDoc(hits)
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}

// capPerDoc drops the chunks past maxPerDoc from any one document. Score order in, score
// order out — it keeps each document's best and lets the freed slots fall to the next
// document, so k still decides the size and this decides the spread.
func capPerDoc(hits []Hit) []Hit {
	seen := map[string]int{}
	kept := hits[:0] // in place: the write index never overtakes the read index
	for _, h := range hits {
		if seen[h.DocPath] >= maxPerDoc {
			continue
		}
		seen[h.DocPath]++
		kept = append(kept, h)
	}
	return kept
}

// stitchNeighbours grows each hit to the chunks immediately before and after it in its own
// document, so the model reads a whole section instead of the 2400-character piece that
// happened to rank. SplitMarkdown cuts at a size, not at a thought, and a chunk that starts
// mid-argument is what makes an answer read as if it skipped a step.
//
// The hits themselves do not change: same count, same order, same ChunkID, DocPath and
// Heading, so a citation still numbers exactly one hit and [n] never moves. Only Content
// grows.
//
// A neighbour that is already a hit in its own right is skipped — two adjacent chunks that
// both ranked would otherwise print the same paragraph under two different citations.
func (s *Store) stitchNeighbours(hits []Hit) ([]Hit, error) {
	if len(hits) == 0 {
		return hits, nil
	}
	type at struct{ doc, ord int }
	ranked := make(map[at]bool, len(hits))
	wanted := make(map[at]bool, 2*len(hits))
	for _, h := range hits {
		ranked[at{h.DocumentID, h.Ord}] = true
	}
	for _, h := range hits {
		for _, ord := range [...]int{h.Ord - 1, h.Ord + 1} {
			if !ranked[at{h.DocumentID, ord}] {
				wanted[at{h.DocumentID, ord}] = true
			}
		}
	}
	if len(wanted) == 0 {
		return hits, nil
	}

	args := make([]any, 0, 2*len(wanted))
	for w := range wanted {
		args = append(args, w.doc, w.ord)
	}
	placeholders := strings.TrimRight(strings.Repeat("(?,?),", len(wanted)), ",")
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT document_id, ord, content FROM chunks WHERE (document_id, ord) IN (%s)`,
		placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	body := make(map[at]string, len(wanted))
	for rows.Next() {
		var w at
		var content string
		if err := rows.Scan(&w.doc, &w.ord, &content); err != nil {
			return nil, err
		}
		body[w] = content
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range hits {
		h := &hits[i]
		var b strings.Builder
		if before, ok := body[at{h.DocumentID, h.Ord - 1}]; ok {
			b.WriteString(before + "\n\n")
		}
		b.WriteString(h.Content)
		if after, ok := body[at{h.DocumentID, h.Ord + 1}]; ok {
			b.WriteString("\n\n" + after)
		}
		h.Content = b.String()
	}
	return hits, nil
}

// scopeFilter restricts a retriever to the chunks under one document path. col is the
// chunk-id column of the table being filtered — `chunk_id` in vec_chunks, `rowid` in
// fts_chunks — and an empty scope adds no clause at all, so the unscoped query is the
// one that always ran.
//
// A scope matches the document itself *or* anything below it, which is what makes one
// control serve both a folder and a single file: picking "booking/calendar" asks its
// whole subtree, picking "booking/calendar/x.md" asks that file.
//
// sqlite-vec applies a `chunk_id IN (…)` constraint before the KNN, so k counts
// matches inside the scope rather than survivors of a global top-k. Verified against
// v0.1.6 in TestScopedSearchRanksWithinTheScope; a version that regressed that would
// turn scoped answers into thin ones.
func scopeFilter(col, scope string) (string, []any) {
	if scope == "" {
		return "", nil
	}
	return ` AND ` + col + ` IN (SELECT c.id FROM chunks c JOIN documents d ON d.id = c.document_id
			WHERE d.path = ? OR d.path LIKE ? ESCAPE '\')`,
		[]any{scope, likeLiteral(scope) + `/%`}
}

// likeLiteral escapes the three characters LIKE treats as syntax, so a document path
// containing one is matched as itself rather than as a pattern.
func likeLiteral(s string) string {
	for _, c := range []string{`\`, `%`, `_`} {
		s = strings.ReplaceAll(s, c, `\`+c)
	}
	return s
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// toFTSQuery turns arbitrary user text into a safe FTS5 OR-query of quoted terms.
func toFTSQuery(text string) string {
	terms := wordRe.FindAllString(text, -1)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(terms))
	for _, t := range terms {
		if len(t) < 2 {
			continue
		}
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, " OR ")
}

// Document is one document in the library, with how much of it is retrievable.
//
// The body is deliberately absent: this is the list, and a library of two hundred documents
// would otherwise send two hundred bodies to a phone to render a table. `Document(path)`
// fetches the one being edited.
type Document struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Alias       string `json:"alias"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Chunks      int    `json:"chunks"`
	Approved    int    `json:"approved"`
	UpdatedAt   string `json:"updated_at"`
}

// Corpus is what the engine actually knows — the answer to "is anything indexed,
// and what?", which is otherwise indistinguishable from a broken retriever.
type Corpus struct {
	Documents []Document `json:"documents"`
	Docs      int        `json:"docs"`
	Chunks    int        `json:"chunks"`
	Approved  int        `json:"approved"`
}

// Corpus lists indexed documents, newest first, capped at limit (<=0 means 200).
func (s *Store) Corpus(limit int) (Corpus, error) {
	if limit <= 0 {
		limit = 200
	}
	var c Corpus
	if err := s.db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM documents WHERE deleted_at IS NULL),
		       (SELECT COUNT(*) FROM chunks),
		       (SELECT COUNT(*) FROM chunks WHERE status = 'approved')`,
	).Scan(&c.Docs, &c.Chunks, &c.Approved); err != nil {
		return c, fmt.Errorf("corpus totals: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT d.path, COALESCE(d.title, ''), COALESCE(d.alias, ''),
		       COALESCE(d.kind, ''), COALESCE(d.description, ''), d.updated_at,
		       COUNT(c.id),
		       COALESCE(SUM(c.status = 'approved'), 0)
		FROM documents d
		LEFT JOIN chunks c ON c.document_id = d.id
		WHERE d.deleted_at IS NULL
		GROUP BY d.id
		ORDER BY d.updated_at DESC, d.path
		LIMIT ?`, limit)
	if err != nil {
		return c, fmt.Errorf("corpus list: %w", err)
	}
	defer rows.Close()

	c.Documents = []Document{} // never nil: this is serialised straight to JSON
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.Path, &d.Title, &d.Alias, &d.Kind, &d.Description,
			&d.UpdatedAt, &d.Chunks, &d.Approved); err != nil {
			return c, err
		}
		c.Documents = append(c.Documents, d)
	}
	return c, rows.Err()
}

// DocumentsMatching counts every live document whose *identity* mentions the term, and
// returns the first `limit` of them. The total is separate from the slice on purpose: a
// count that only counts what it can show is the bug this exists to fix.
//
// Identity, not text: path, title, alias and kind — the four things a person files a document
// under. Retrieval cannot answer "how many X are there" and never could, for two reasons that
// are both structural. It ranks and truncates, so a total is not a thing it produces; and
// `fts_chunks` indexes a chunk's content and heading, so a document called
// `decision-4-auth.md` whose text never says "decision" is invisible to that word. Measured:
// on a 52-document corpus, "có bao nhiêu decision" answered with a table of unrelated notes
// and named none of the seven decisions.
//
// LIKE with no index behind it, deliberately. This runs once per *library* question, never on
// the retrieval path, and a corpus that makes a scan of four short columns slow is one that
// outgrew the 200-document list on the screen long before it outgrew this.
func (s *Store) DocumentsMatching(term string, limit int) (int, []Document, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + strings.ToLower(term) + "%"
	where := `FROM documents d WHERE d.deleted_at IS NULL AND (
		    lower(d.path) LIKE ? OR lower(COALESCE(d.title,'')) LIKE ?
		 OR lower(COALESCE(d.alias,'')) LIKE ? OR lower(COALESCE(d.kind,'')) LIKE ?)`
	args := []any{like, like, like, like}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) `+where, args...).Scan(&total); err != nil {
		return 0, nil, fmt.Errorf("counting %q: %w", term, err)
	}
	rows, err := s.db.Query(`
		SELECT d.path, COALESCE(d.title,''), COALESCE(d.alias,''), COALESCE(d.kind,''),
		       COALESCE(d.description,''), d.updated_at,
		       (SELECT COUNT(*) FROM chunks c WHERE c.document_id = d.id),
		       (SELECT COALESCE(SUM(c.status='approved'),0) FROM chunks c WHERE c.document_id = d.id)
		`+where+` ORDER BY d.path LIMIT ?`, append(args, limit)...)
	if err != nil {
		return 0, nil, fmt.Errorf("listing %q: %w", term, err)
	}
	defer rows.Close()

	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.Path, &d.Title, &d.Alias, &d.Kind, &d.Description,
			&d.UpdatedAt, &d.Chunks, &d.Approved); err != nil {
			return 0, nil, err
		}
		out = append(out, d)
	}
	return total, out, rows.Err()
}

// PathWithBody finds a live document holding exactly this text at some other path, so a
// second copy of one fact can be refused at the door it arrives through.
//
// Raw bodies, compared as stored. That is what makes it free: the case it exists for is the
// same file imported twice — byte-identical — and matching *near*-identical text would mean
// a hash column, a migration, a backfill, and a judgement about how near is near. A document
// edited by one character is a different document, and the person who edited it knows that.
//
// `except` is the path being written, so re-importing a document over itself is an update
// rather than its own twin.
func (s *Store) PathWithBody(body, except string) (string, bool, error) {
	var p string
	err := s.db.QueryRow(
		`SELECT path FROM documents
		 WHERE deleted_at IS NULL AND body = ? AND path <> ? LIMIT 1`, body, except).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("looking for a duplicate of %s: %w", except, err)
	}
	return p, true, nil
}

// RecentDocuments lists what changed last, newest first, optionally within one folder.
//
// Its own query rather than Corpus() plus a filter in Go, and the difference is not style:
// Corpus caps at 200, so filtering "qa/" out of that page would quietly return three
// confirmed answers on a corpus where the newest 200 documents happen to hold three. A
// number that is short for a reason nobody can see is worse than no number.
//
// `prefix` is matched as a folder, so it carries its own separator: "qa" would also match
// "qawhatever/x.md". Empty means the whole library. Retrieval cannot answer this at all —
// it ranks by meaning and keywords, and "recently" carries neither — which is why the
// engine reads rows here instead of embedding a question about time.
func (s *Store) RecentDocuments(prefix string, limit int) ([]Document, error) {
	if limit <= 0 {
		limit = 10
	}
	like := "%"
	if prefix != "" {
		like = strings.TrimSuffix(prefix, "/") + "/%"
	}
	rows, err := s.db.Query(`
		SELECT d.path, COALESCE(d.title, ''), COALESCE(d.alias, ''),
		       COALESCE(d.kind, ''), COALESCE(d.description, ''), d.updated_at,
		       COUNT(c.id),
		       COALESCE(SUM(c.status = 'approved'), 0)
		FROM documents d
		LEFT JOIN chunks c ON c.document_id = d.id
		WHERE d.deleted_at IS NULL AND d.path LIKE ?
		GROUP BY d.id
		ORDER BY d.updated_at DESC, d.path
		LIMIT ?`, like, limit)
	if err != nil {
		return nil, fmt.Errorf("recent documents: %w", err)
	}
	defer rows.Close()

	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.Path, &d.Title, &d.Alias, &d.Kind, &d.Description,
			&d.UpdatedAt, &d.Chunks, &d.Approved); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Document reads one document whole, body included — what the edit form needs and the list
// deliberately does not carry. A removed document still reads, because that is the only way
// back for one: the trash is a column now, not a directory.
func (s *Store) Document(path string) (Doc, bool, error) {
	var d Doc
	err := s.db.QueryRow(`
		SELECT path, COALESCE(title,''), COALESCE(alias,''), COALESCE(kind,''),
		       COALESCE(description,''), COALESCE(body,'')
		FROM documents WHERE path=?`, path,
	).Scan(&d.Path, &d.Title, &d.Alias, &d.Kind, &d.Description, &d.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return Doc{}, false, nil
	}
	if err != nil {
		return Doc{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	return d, true, nil
}

// RemoveDocument takes one document out of retrieval and keeps its text. Reports whether it
// was there.
//
// The vectors and chunks go, which is the whole request: a removed document must stop
// answering questions the moment it is removed. The row stays with `deleted_at` set, so the
// body is still there for whoever has the database — the same deal the .trash/ directory
// offered before the database became the source of truth, and the only way back that exists
// now that nothing on disk holds a second copy.
//
// The three writes are one transaction because a half-removed document is worse than either
// state: chunks with no vectors are a retriever that ranks nothing, and vectors with no
// chunk are a KNN candidate that resolves to nothing.
func (s *Store) RemoveDocument(path string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var id sql.NullInt64
	if err := tx.QueryRow(
		`SELECT id FROM documents WHERE path=? AND deleted_at IS NULL`, path).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if !id.Valid {
		return false, nil
	}
	for _, q := range []string{
		`DELETE FROM vec_chunks WHERE chunk_id IN (SELECT id FROM chunks WHERE document_id=?)`,
		`DELETE FROM chunks WHERE document_id=?`,
		`UPDATE documents SET deleted_at=datetime('now') WHERE id=?`,
	} {
		if _, err := tx.Exec(q, id.Int64); err != nil {
			return false, fmt.Errorf("removing %s: %w", path, err)
		}
	}
	return true, tx.Commit()
}
