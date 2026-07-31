package rag

import "knowledge-engine/internal/db"

// defaultContextShare is how much of a model's window the retrieved sections may take when
// CONTEXT_SHARE says nothing.
//
// Half, against the thread's 0.35, leaves about a sixth of the window for the completion —
// which is the right way round: the sections are why an answer is grounded at all, and the
// answer is shorter than the documents behind it. The two shares are separate knobs because
// they are separate trades. Tuning how much a conversation may remember should not silently
// change how much of the corpus an answer is built from.
const defaultContextShare = 0.5

// contextBudget is how many characters of retrieved context this model's window affords, in
// the same crude perToken estimate replay already uses for the thread.
//
// Zero means the operator never said what the window is, and then nothing here applies:
// TOP_K decides the count exactly as it always has. A retrieval width that depended on an
// optional display knob being filled in would make an unconfigured instance answer worse
// with no way to tell.
func (e *Engine) contextBudget(model string) int {
	window := e.window(model)
	if window <= 0 {
		return 0
	}
	share := e.contextShare
	if share <= 0 {
		share = defaultContextShare
	}
	return int(float64(window)*share) * perToken
}

// trimToBudget keeps the best-ranked hits that fit, and always at least the first one — a
// section too big for the budget on its own is still the best answer there is, and dropping
// it would answer from nothing rather than from too much.
func trimToBudget(hits []db.Hit, budget int) []db.Hit {
	spent := 0
	for i, h := range hits {
		spent += len(h.Content)
		if i > 0 && spent > budget {
			return hits[:i]
		}
	}
	return hits
}

// retrieve is the one seam Answer calls through for context: what to ask the store for, and
// how much of the answer to keep once it replies.
//
// TOP_K used to be the whole story, and it was a fixed six against a window that holds
// forty: six sections is about three per cent of a 128k model, so the instance paid for a
// reader and used a skim. When the operator has said what the window is, retrieval asks for
// everything the candidate pool holds, reads each hit together with its neighbours, and
// keeps what fits — the count then follows the model rather than a number nobody re-tuned.
//
// It is a function of its own rather than four more lines in Answer because Answer was
// already at gocyclo's ceiling once, and the fix that time was extracting serveCached, not
// raising the limit.
func (e *Engine) retrieve(qEmb []float32, query, model, scope string) ([]db.Hit, Recall, error) {
	budget := e.contextBudget(model)
	k := e.topK
	if budget > 0 {
		k = db.CandidatePool
	}
	hits, err := e.store.Search(qEmb, query, k, scope, budget > 0)
	if err != nil {
		return nil, Recall{}, err
	}
	offered := len(hits)
	if budget > 0 {
		hits = trimToBudget(hits, budget)
	}
	return hits, Recall{Kept: len(hits), Offered: offered}, nil
}
