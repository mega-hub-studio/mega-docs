package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

type Store struct {
	db  *sql.DB
	dim int
}

// Hit is one retrieved chunk with provenance for citation.
type Hit struct {
	ChunkID int
	DocPath string
	Heading string
	Content string
	Status  string
	Score   float64
}

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
	_, err := s.db.Exec(q)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// UpsertDocument replaces a document and all its chunks (idempotent re-ingest).
func (s *Store) UpsertDocument(path, title string) (int64, error) {
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
	res, err := tx.Exec(
		`INSERT INTO documents(path,title,version,updated_at)
		 VALUES(?,?,1,datetime('now'))
		 ON CONFLICT(path) DO UPDATE SET title=excluded.title,
		   version=version+1, updated_at=datetime('now')`,
		path, title)
	if err != nil {
		return 0, err
	}
	_ = res
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

	res, err := tx.Exec(
		`INSERT INTO chunks(document_id,heading,content,ord) VALUES(?,?,?,?)`,
		docID, heading, content, ord)
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
// Approved chunks get a small score boost (Phase-1 SoT behaviour, no-op while all draft).
func (s *Store) Search(qEmb []float32, qText string, k int) ([]Hit, error) {
	const pool = 40 // candidates pulled from each retriever before fusion
	const rrfK = 60.0

	// 1) Vector candidates
	blob, err := sqlite_vec.SerializeFloat32(qEmb)
	if err != nil {
		return nil, err
	}
	vrows, err := s.db.Query(
		`SELECT chunk_id FROM vec_chunks WHERE embedding MATCH ? AND k = ? ORDER BY distance`,
		blob, pool)
	if err != nil {
		return nil, fmt.Errorf("vec search: %w", err)
	}
	var vecOrder []int
	for vrows.Next() {
		var id int
		if err := vrows.Scan(&id); err != nil {
			vrows.Close()
			return nil, err
		}
		vecOrder = append(vecOrder, id)
	}
	vrows.Close()

	// 2) Keyword candidates (BM25)
	var ftsOrder []int
	if match := toFTSQuery(qText); match != "" {
		frows, err := s.db.Query(
			`SELECT rowid FROM fts_chunks WHERE fts_chunks MATCH ? ORDER BY rank LIMIT ?`,
			match, pool)
		if err == nil {
			for frows.Next() {
				var id int
				if err := frows.Scan(&id); err == nil {
					ftsOrder = append(ftsOrder, id)
				}
			}
			frows.Close()
		}
	}

	// 3) Reciprocal Rank Fusion
	score := map[int]float64{}
	for rank, id := range vecOrder {
		score[id] += 1.0 / (rrfK + float64(rank))
	}
	for rank, id := range ftsOrder {
		score[id] += 1.0 / (rrfK + float64(rank))
	}
	if len(score) == 0 {
		return nil, nil
	}

	ids := make([]int, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}

	// 4) Load chunk bodies + provenance
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT c.id, d.path, c.heading, c.content, c.status
		 FROM chunks c JOIN documents d ON d.id = c.document_id
		 WHERE c.id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ChunkID, &h.DocPath, &h.Heading, &h.Content, &h.Status); err != nil {
			return nil, err
		}
		h.Score = score[h.ChunkID]
		if h.Status == "approved" {
			h.Score *= 1.2 // SoT priority; harmless while everything is draft
		}
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
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

// Document is one indexed source file, with how much of it is retrievable.
type Document struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	Chunks    int    `json:"chunks"`
	Approved  int    `json:"approved"`
	UpdatedAt string `json:"updated_at"`
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
		SELECT (SELECT COUNT(*) FROM documents),
		       (SELECT COUNT(*) FROM chunks),
		       (SELECT COUNT(*) FROM chunks WHERE status = 'approved')`,
	).Scan(&c.Docs, &c.Chunks, &c.Approved); err != nil {
		return c, fmt.Errorf("corpus totals: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT d.path, COALESCE(d.title, ''), d.updated_at,
		       COUNT(c.id),
		       COALESCE(SUM(c.status = 'approved'), 0)
		FROM documents d
		LEFT JOIN chunks c ON c.document_id = d.id
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
		if err := rows.Scan(&d.Path, &d.Title, &d.UpdatedAt, &d.Chunks, &d.Approved); err != nil {
			return c, err
		}
		c.Documents = append(c.Documents, d)
	}
	return c, rows.Err()
}
