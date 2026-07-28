package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Cached is one answer the engine already paid for. It is both the cache row and
// one line of the History panel — the same fact serves both, so there is no second
// table recording "questions people asked".
type Cached struct {
	Question string `json:"question"`
	// Scope is the part of the corpus the answer was retrieved from — "" is the whole
	// of it. Not a column: it lives inside the key (see cacheKey), so replaying a
	// scoped answer can restore the scope it was answered under.
	Scope     string          `json:"scope"`
	Answer    string          `json:"answer"`
	Citations json.RawMessage `json:"citations"` // opaque here: db must not import rag
	Hits      int             `json:"hits"`
	At        string          `json:"at"`
}

// keep caps the cache. 200 answers is more than a team asks between re-indexes, and it keeps
// the one-file story honest: a database small enough to copy in a second is a database nobody
// has to plan around.
const keep = 200

// Sig identifies the current state of the corpus. Any ingest changes the document
// count or its timestamp, and any confirm ingests — so a cached answer can never
// outlive the documents it cited. Cheap enough to call per request, but not because it is
// indexed — there is no index on documents.updated_at, so the MAX scans the documents table.
// That is fine while one row is one document and the count is in the hundreds; it is the
// first thing to look at if Sig() ever shows up in a profile.
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

// cacheKey is the identity of a cached answer. The same words asked inside a folder
// are a different question: answering one from the other's row would cite documents
// the asker deliberately left out.
//
// The scope belongs in the key and not in the corpus signature, even though both
// invalidate. A signature is *pruned* when it changes — scoping it would make every
// scope change wipe every other scope's answers, and the panel that exists to say
// "this one is free" would empty on each click. An unscoped question keeps the bare
// normalised form, so rows cached before scopes existed are still served.
func cacheKey(scope, question string) string {
	if scope == "" {
		return normalise(question)
	}
	return scope + "\x1f" + normalise(question)
}

// scopeOf reads the scope back out of a key. The stored answer is the only record of
// what it was retrieved from, and a History row has to replay under the same scope or
// the "free" it advertises is a different answer.
func scopeOf(key string) string {
	scope, _, found := strings.Cut(key, "\x1f")
	if !found {
		return ""
	}
	return scope
}

// Cached returns a stored answer for this question in this scope, and counts the hit.
// A miss is (Cached{}, false, nil) — not an error, since most questions are new.
func (s *Store) Cached(sig, scope, question string) (Cached, bool, error) {
	norm := cacheKey(scope, question)
	var c Cached
	c.Scope = scope
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
		cacheKey(c.Scope, c.Question), c.Question, c.Answer, string(c.Citations), sig); err != nil {
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
		`SELECT q_norm, question, answer, citations, hits, used_at FROM answers
		 WHERE corpus_sig=? ORDER BY used_at DESC LIMIT ?`, sig, limit)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer rows.Close()

	out := []Cached{} // never nil: serialised straight to JSON
	for rows.Next() {
		var c Cached
		var key string
		var cites []byte
		if err := rows.Scan(&key, &c.Question, &c.Answer, &cites, &c.Hits, &c.At); err != nil {
			return nil, err
		}
		c.Scope = scopeOf(key)
		c.Citations = cites
		out = append(out, c)
	}
	return out, rows.Err()
}
