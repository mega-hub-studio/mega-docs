package db

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Ticket is one gap in the documents, on its way to becoming part of them.
//
// The lifecycle is the status field, and nothing else: there is no separate
// assignee, priority or comment thread, because a queue a BA clears in one sitting
// does not need one.
type Ticket struct {
	ID        int64  `json:"id"`
	Question  string `json:"question"`
	Miss      string `json:"miss"`   // what the engine answered instead — the evidence
	Status    string `json:"status"` // see the constants below
	Answer    string `json:"answer"`
	Note      string `json:"note"`
	DocPath   string `json:"doc"` // the document a confirm created
	AskedAt   string `json:"asked_at"`
	UpdatedAt string `json:"updated_at"`
}

// The four states. Every one of them is reached by exactly one action, so the UI
// can render the state machine without inventing labels for states nobody enters.
const (
	StatusOpen      = "open"      // a DEV filed it
	StatusAnswered  = "answered"  // a BA saved a draft; not yet retrievable
	StatusConfirmed = "confirmed" // indexed + approved; the next DEV gets a citation
	StatusRejected  = "rejected"  // not a documentation gap
)

// ErrNoTicket is returned instead of sql.ErrNoRows, so callers don't import
// database/sql to tell "gone" from "broken".
var ErrNoTicket = errors.New("no such ticket")

// Queue is the whole BA view in one response: the tickets plus the counts the
// header badge needs, so a queue screen is one request.
type Queue struct {
	Tickets   []Ticket `json:"tickets"`
	Open      int      `json:"open"`
	Answered  int      `json:"answered"`
	Confirmed int      `json:"confirmed"`
	Rejected  int      `json:"rejected"`
}

const ticketCols = `id, question, miss, status, answer, note, doc_path, asked_at, updated_at`

// OpenTicket files a gap, or returns the ticket that already covers it.
//
// Returning the existing one is the point: three devs hitting the same gap this
// morning is one question for the BA, not three. The second caller still gets a
// ticket back, so the UI can show them where their question went.
func (s *Store) OpenTicket(question, miss string) (Ticket, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return Ticket{}, errors.New("empty question")
	}
	norm := normalise(q)

	if t, err := s.ticketByNorm(norm); err == nil {
		return t, nil
	} else if !errors.Is(err, ErrNoTicket) {
		return Ticket{}, err
	}

	res, err := s.db.Exec(
		`INSERT INTO tickets(question, q_norm, miss) VALUES(?,?,?)`, q, norm, strings.TrimSpace(miss))
	if err != nil {
		// Lost a race against another ask of the same question — that ticket is
		// the right answer here, not an error.
		if t, e := s.ticketByNorm(norm); e == nil {
			return t, nil
		}
		return Ticket{}, err
	}
	id, _ := res.LastInsertId()
	return s.Ticket(id)
}

// Draft saves a BA's answer without publishing it. A long answer typed on a phone
// must survive a backgrounded tab, and a colleague can pick it up.
func (s *Store) Draft(id int64, answer string) (Ticket, error) {
	return s.update(id, `UPDATE tickets SET answer=?, status=?, updated_at=datetime('now')
		WHERE id=? AND status IN ('open','answered')`, answer, StatusAnswered, id)
}

// Confirm records that this answer is now part of the corpus at docPath. The
// indexing itself belongs to the engine; this is only the bookkeeping.
func (s *Store) Confirm(id int64, answer, docPath string) (Ticket, error) {
	return s.update(id, `UPDATE tickets SET answer=?, doc_path=?, status=?, note='',
		updated_at=datetime('now') WHERE id=? AND status IN ('open','answered')`,
		answer, docPath, StatusConfirmed, id)
}

// Reject closes a ticket that isn't a documentation gap. The note is why — an
// unexplained dismissal is indistinguishable from the queue eating the question.
func (s *Store) Reject(id int64, note string) (Ticket, error) {
	return s.update(id, `UPDATE tickets SET note=?, status=?, updated_at=datetime('now')
		WHERE id=? AND status IN ('open','answered')`, strings.TrimSpace(note), StatusRejected, id)
}

// update applies one transition and reports the ticket as it now stands. A zero
// row count means the ticket is missing or already settled — the caller can't tell
// those apart from an error, so look it up and say which.
func (s *Store) update(id int64, query string, args ...any) (Ticket, error) {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return Ticket{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t, err := s.Ticket(id)
		if err != nil {
			return Ticket{}, err
		}
		return t, fmt.Errorf("ticket %d is already %s", id, t.Status)
	}
	return s.Ticket(id)
}

// Ticket returns one ticket by id, or ErrNoTicket when there is none — so a caller can
// tell "gone" from "broken" without inspecting the error text.
func (s *Store) Ticket(id int64) (Ticket, error) {
	return scanTicket(s.db.QueryRow(`SELECT `+ticketCols+` FROM tickets WHERE id=?`, id))
}

func (s *Store) ticketByNorm(norm string) (Ticket, error) {
	return scanTicket(s.db.QueryRow(
		`SELECT `+ticketCols+` FROM tickets WHERE q_norm=? AND status IN ('open','answered')`, norm))
}

func scanTicket(row *sql.Row) (Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.Question, &t.Miss, &t.Status, &t.Answer, &t.Note,
		&t.DocPath, &t.AskedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNoTicket
	}
	return t, err
}

// Queue lists tickets with the work first: open, then drafts, then what's settled.
// Within a group, newest first.
func (s *Store) Queue(limit int) (Queue, error) {
	if limit <= 0 {
		limit = 100
	}
	var q Queue
	if err := s.db.QueryRow(`SELECT
			COALESCE(SUM(status='open'),0),      COALESCE(SUM(status='answered'),0),
			COALESCE(SUM(status='confirmed'),0), COALESCE(SUM(status='rejected'),0)
		FROM tickets`).Scan(&q.Open, &q.Answered, &q.Confirmed, &q.Rejected); err != nil {
		return q, fmt.Errorf("ticket counts: %w", err)
	}

	rows, err := s.db.Query(`SELECT `+ticketCols+` FROM tickets
		ORDER BY CASE status WHEN 'open' THEN 0 WHEN 'answered' THEN 1 ELSE 2 END,
		         updated_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return q, fmt.Errorf("ticket list: %w", err)
	}
	defer rows.Close()

	q.Tickets = []Ticket{} // never nil: this is serialised straight to JSON
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.Question, &t.Miss, &t.Status, &t.Answer, &t.Note,
			&t.DocPath, &t.AskedAt, &t.UpdatedAt); err != nil {
			return q, err
		}
		q.Tickets = append(q.Tickets, t)
	}
	return q, rows.Err()
}

// Approve marks every chunk of a document as approved — the SoT boost in Search()
// that has been dormant since it was written. A BA-confirmed answer is the one
// thing in the corpus a human vouched for by name, so it outranks a tie.
func (s *Store) Approve(path string) error {
	_, err := s.db.Exec(
		`UPDATE chunks SET status='approved'
		 WHERE document_id = (SELECT id FROM documents WHERE path=?)`, path)
	return err
}

var spaceRe = regexp.MustCompile(`\s+`)

// normalise is the "same question" rule, shared by the ticket dedupe and the answer
// cache: case and spacing don't count, anything else does. Deliberately not
// semantic — matching by meaning needs a vector and a threshold, and a wrong match
// there serves a confidently unrelated answer.
func normalise(q string) string {
	return spaceRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(q)), " ")
}
