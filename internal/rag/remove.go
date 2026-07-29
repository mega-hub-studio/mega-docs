package rag

import (
	"context"
	"fmt"
)

// Removing a document, which is the other half of the write surface Upload opened.
//
// It is a *soft* delete, and after the inversion that is the only safety net there is: the
// database holds the documents, so a hard delete would destroy the original — including a
// BA-confirmed answer, which cost a person's time to write and which nothing can
// reconstruct. The rows that make it answerable go; the row that holds its text stays.

// Removed is what a removal did, per document.
type Removed struct {
	Path string `json:"path"` // the identity it was stored and cited under
}

// Remove takes one document out of retrieval and keeps its text.
//
// It is one call now instead of a file move plus an index clean, and that is the inversion
// paying for itself: the old version had three outcomes to reason about, and the worst of
// them — rows deleted, file left behind — was a delete that the next `ingest docs` silently
// undid. There is no file, so there is no way back into the index except an import.
//
// What "removed" means here: the chunks and vectors are gone, so the document stops
// answering questions immediately, and the row stays with `deleted_at` set, so its body is
// still there for whoever has the database. That is the trash, as a column.
func (e *Engine) Remove(_ context.Context, name string) (Removed, error) {
	// readPath: a confirmed answer can be taken out of retrieval like any other document —
	// it is one library, and the qa/ rule is about what may be *created* there.
	rel, err := readPath(name)
	if err != nil {
		return Removed{}, err
	}
	existed, err := e.store.RemoveDocument(rel)
	if err != nil {
		return Removed{}, err
	}
	if !existed {
		return Removed{}, fmt.Errorf("%s is not in the library", rel)
	}
	return Removed{Path: rel}, nil
}
