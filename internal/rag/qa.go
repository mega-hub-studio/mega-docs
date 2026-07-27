package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"knowledge-engine/internal/db"
)

// The QA loop: a question the documents couldn't answer becomes part of them.
//
//	DEV asks → answer is wrong or missing → Ask BA        (ticket: open)
//	BA answers → Confirm into knowledge                   (ticket: confirmed)
//	next DEV asks → retrieved, cited, free the second time
//
// A confirm writes a real markdown file into the corpus directory and indexes that
// file — it does not stash prose in the database. That keeps one property worth
// protecting: the database is derived. `ingest docs` rebuilds it, and the answers a
// BA vouched for live in git with everything else, not in a blob nobody can review.

// QADir is where confirmed answers land, relative to the corpus directory. A
// separate folder because these are answers to questions, not authored documents —
// useful to review as a set, and obvious in a diff.
const QADir = "qa"

// Queue is the BA's view of the loop.
func (e *Engine) Queue(limit int) (db.Queue, error) { return e.store.Queue(limit) }

// OpenTicket files the gap a DEV just hit. miss is what the engine answered
// instead — evidence for the BA, and the fastest way to see whether the corpus is
// wrong or merely silent.
func (e *Engine) OpenTicket(question, miss string) (db.Ticket, error) {
	return e.store.OpenTicket(question, miss)
}

// Draft saves a BA's answer without publishing it.
func (e *Engine) Draft(id int64, answer string) (db.Ticket, error) {
	return e.store.Draft(id, answer)
}

// Reject closes a ticket that isn't a documentation gap.
func (e *Engine) Reject(id int64, note string) (db.Ticket, error) {
	return e.store.Reject(id, note)
}

// Confirm publishes a BA's answer: write the file, index it, mark its chunks
// approved, close the ticket.
//
// Order matters. The file is written first because it is the source of truth — if
// indexing then fails, `ingest docs` still picks it up. The ticket closes last, so
// a failure anywhere leaves it in the queue instead of silently swallowing the work.
func (e *Engine) Confirm(ctx context.Context, id int64, answer string) (db.Ticket, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return db.Ticket{}, fmt.Errorf("an answer is required to confirm ticket %d", id)
	}
	t, err := e.store.Ticket(id)
	if err != nil {
		return db.Ticket{}, err
	}
	if t.Status == db.StatusConfirmed {
		return t, fmt.Errorf("ticket %d is already in the knowledge base (%s)", id, t.DocPath)
	}

	rel := filepath.Join(QADir, fmt.Sprintf("ticket-%d.md", id))
	if err := e.writeDoc(rel, qaMarkdown(t.Question, answer)); err != nil {
		return t, err
	}
	if _, err := e.ingest(ctx, rel, t.Question, qaMarkdown(t.Question, answer)); err != nil {
		return t, fmt.Errorf("indexing %s: %w", rel, err)
	}
	// Approved chunks win ties in retrieval. This is the one part of the corpus a
	// named human vouched for, so that boost is finally earned.
	if err := e.store.Approve(rel); err != nil {
		return t, err
	}
	return e.store.Confirm(id, answer, rel)
}

// writeDoc puts a file in the corpus directory, refusing to leave the tree.
func (e *Engine) writeDoc(rel, content string) error {
	if e.corpusDir == "" {
		return fmt.Errorf("no corpus directory configured: set CORPUS_DIR to the folder ingest reads")
	}
	full := filepath.Join(e.corpusDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("%s: %w", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", full, err)
	}
	return nil
}

// qaMarkdown is the shape of a confirmed answer on disk: the question as the
// heading, so retrieval's breadcrumb reads as the question it answers.
func qaMarkdown(question, answer string) string {
	return fmt.Sprintf("# %s\n\n%s\n", strings.TrimSpace(question), strings.TrimSpace(answer))
}

// History lists the answers still free to replay. Empty rather than an error when
// the corpus signature can't be read: history is a convenience, not the product.
func (e *Engine) History(limit int) ([]db.Cached, error) {
	sig, err := e.store.Sig()
	if err != nil {
		return []db.Cached{}, nil
	}
	return e.store.History(sig, limit)
}
