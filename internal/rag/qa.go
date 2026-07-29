package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	"knowledge-engine/internal/db"
)

// The QA loop: a question the documents couldn't answer becomes part of them.
//
//	DEV asks → answer is wrong or missing → Ask BA        (ticket: open)
//	BA answers → Confirm into knowledge                   (ticket: confirmed)
//	next DEV asks → retrieved, cited, free the second time
//
// A confirm writes the answer straight into the database as a document row and indexes
// it in the same call — nothing reaches disk. The path stays `qa/ticket-N.md` because
// that is what a citation prints and what a scope matches, not because a file exists,
// and the row *is* the document like every other one (invariant 1).

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

	rel := path.Join(QADir, fmt.Sprintf("ticket-%d.md", id))
	body := qaMarkdown(t.Question, answer)
	// The .md path is kept even though nothing writes a file: it is what a citation reads,
	// what a scope matches, and what the ticket stores as its DocPath. A confirmed answer
	// arriving at another kind of address would be a second identity for a document.
	if _, err := e.ingest(ctx, db.Doc{
		Path: rel, Title: t.Question, Kind: qaKind,
		Description: "Confirmed answer to a question the documents did not cover.",
		Body:        body,
	}); err != nil {
		return t, fmt.Errorf("indexing %s: %w", rel, err)
	}
	// Approved chunks win ties in retrieval. This is the one part of the corpus a
	// named human vouched for, so that boost is finally earned.
	if err := e.store.Approve(rel); err != nil {
		return t, err
	}
	return e.store.Confirm(id, answer, rel)
}

// qaKind is what a confirmed answer files itself under, so the library can show a BA which
// of its documents the loop produced rather than a person importing.
const qaKind = "answer"

// qaMarkdown is the shape of a confirmed answer: the question as the heading, so
// retrieval's breadcrumb reads as the question it answers.
func qaMarkdown(question, answer string) string {
	return fmt.Sprintf("# %s\n\n%s\n", strings.TrimSpace(question), strings.TrimSpace(answer))
}

// promptSig fingerprints the instructions an answer was produced under. Computed
// from the constant itself rather than a version anyone has to remember to bump —
// an editor who forgets would leave every cached answer claiming rules it was never
// given.
var promptSig = func() string {
	sum := sha256.Sum256([]byte(systemPrompt))
	return hex.EncodeToString(sum[:4])
}()

// sig identifies everything a cached answer depends on: the corpus it cited, the
// model that wrote it, and the rules it was written under. The store only knows the
// corpus, so the other two are appended here — without them, changing CHAT_MODEL or
// the prompt keeps serving answers from the old one until something happens to
// re-index, which reads as the setting having no effect.
func (e *Engine) sig() (string, error) {
	s, err := e.store.Sig()
	if err != nil {
		return "", err
	}
	return s + "|" + e.ai.ChatModel + "|" + promptSig, nil
}

// History lists the answers still free to replay. Empty rather than an error when
// the corpus signature can't be read: history is a convenience, not the product.
func (e *Engine) History(limit int) ([]db.Cached, error) {
	sig, err := e.sig()
	if err != nil {
		//nolint:nilerr // documented above: an unreadable signature means "no history",
		// never a failed request. The panel is a convenience, the answer is the product.
		return []db.Cached{}, nil
	}
	return e.store.History(sig, limit)
}
