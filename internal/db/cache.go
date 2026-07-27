package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Cached is one answer the engine already paid for. It is both the cache row and
// one line of the History panel — the same fact serves both, so there is no second
// table recording "questions people asked".
type Cached struct {
	Question  string          `json:"question"`
	Answer    string          `json:"answer"`
	Citations json.RawMessage `json:"citations"` // opaque here: db must not import rag
	Hits      int             `json:"hits"`
	At        string          `json:"at"`
}

// keep caps the cache. 200 answers is more than a team asks between re-indexes,
// and it keeps the one-file backup story honest.
const keep = 200

// Sig identifies the current state of the corpus. Any ingest changes the document
// count or its timestamp, and any confirm ingests — so a cached answer can never
// outlive the documents it cited. Cheap enough to call per request (two counts and
// a MAX over indexed columns).
func (s *Store) Sig() (string, error) {
	var docs, chunks int
	var newest sql.NullString
	err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM documents),
		(SELECT COUNT(*) FROM chunks), (SELECT MAX(updated_at) FROM documents)`).
		Scan(&docs, &chunks, &newest)
	if err != nil {
		return "", fmt.Errorf("corpus sig: %w", err)
	}
	return fmt.Sprintf("%d:%d:%s", docs, chunks, newest.String), nil
}

// Cached returns a stored answer for this question, and counts the hit. A miss is
// (Cached{}, false, nil) — not an error, since most questions are new.
func (s *Store) Cached(sig, question string) (Cached, bool, error) {
	norm := normalise(question)
	var c Cached
	var cites []byte // database/sql won't scan into json.RawMessage directly
	err := s.db.QueryRow(
		`SELECT question, answer, citations, hits, used_at FROM answers
		 WHERE q_norm=? AND corpus_sig=?`, norm, sig).
		Scan(&c.Question, &c.Answer, &cites, &c.Hits, &c.At)
	c.Citations = cites
	if errors.Is(err, sql.ErrNoRows) {
		return Cached{}, false, nil
	}
	if err != nil {
		return Cached{}, false, err
	}
	// Best-effort: a failed counter must not cost the user their free answer.
	_, _ = s.db.Exec(
		`UPDATE answers SET hits=hits+1, used_at=datetime('now') WHERE q_norm=?`, norm)
	c.Hits++
	return c, true, nil
}

// Cache stores an answer under the current corpus signature, and drops everything
// the corpus has outgrown. Pruning here rather than on a timer means a re-index
// clears the stale cache as a side effect of the next question.
func (s *Store) Cache(sig string, c Cached) error {
	if len(c.Citations) == 0 {
		c.Citations = json.RawMessage("[]")
	}
	if _, err := s.db.Exec(
		`INSERT INTO answers(q_norm, question, answer, citations, corpus_sig)
		 VALUES(?,?,?,?,?)
		 ON CONFLICT(q_norm) DO UPDATE SET answer=excluded.answer,
		   citations=excluded.citations, corpus_sig=excluded.corpus_sig,
		   used_at=datetime('now')`,
		normalise(c.Question), c.Question, c.Answer, string(c.Citations), sig); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM answers WHERE corpus_sig <> ?
		OR q_norm NOT IN (SELECT q_norm FROM answers ORDER BY used_at DESC LIMIT ?)`, sig, keep)
	return err
}

// History lists the answers still free to replay, most recently used first. Rows
// from an older corpus are excluded rather than shown greyed out: an entry that
// looks replayable and isn't is worse than one that isn't there.
func (s *Store) History(sig string, limit int) ([]Cached, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT question, answer, citations, hits, used_at FROM answers
		 WHERE corpus_sig=? ORDER BY used_at DESC LIMIT ?`, sig, limit)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer rows.Close()

	out := []Cached{} // never nil: serialised straight to JSON
	for rows.Next() {
		var c Cached
		var cites []byte
		if err := rows.Scan(&c.Question, &c.Answer, &cites, &c.Hits, &c.At); err != nil {
			return nil, err
		}
		c.Citations = cites
		out = append(out, c)
	}
	return out, rows.Err()
}
