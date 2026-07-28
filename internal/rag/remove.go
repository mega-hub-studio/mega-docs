package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Removing a document, which is the other half of the write surface Upload opened.
//
// It is a *soft* delete, and the reason is invariant 1: the corpus directory is the source
// of truth and the database is derived from it. A hard delete would therefore be the one
// operation in this system that destroys the original rather than something rebuildable —
// including a BA-confirmed answer in qa/, which cost a person's time to write and which
// `ingest` can never reconstruct.
//
// So the file moves to CORPUS_DIR/.trash/ and the derived rows go. The elegant part is that
// this needs no exclusion rule anywhere: SafePath already refuses a hidden segment, so
// .trash/ is unreachable as a document path and a re-ingest cannot pull the file back in.
// Recovery is `mv` — by whoever has the disk, not whoever has the password.

// Removed is what a removal did, per document.
type Removed struct {
	Path string `json:"path"` // the identity it was stored and cited under
	// Trash is where the file went, relative to the corpus directory. Empty when the
	// document was indexed but its file was already gone — the index is still cleaned,
	// because a citation pointing at a file nobody can open is worse than no row.
	Trash string `json:"trash"`
}

// TrashDir is the folder a removed document is moved to, inside the corpus. Hidden, which
// is what keeps it out of the index without a second rule to remember.
const TrashDir = ".trash"

// Remove takes one document out of the index and moves its file to the trash.
//
// The order matters. The file moves first: if that fails there is nothing to undo, whereas
// deleting the rows first and then failing to move the file leaves a document on disk that
// the next `ingest docs` silently resurrects — a delete that undoes itself, which is the
// worst of the three possible outcomes.
func (e *Engine) Remove(_ context.Context, name string) (Removed, error) {
	rel, err := SafePath(name)
	if err != nil {
		return Removed{}, err
	}
	if e.corpusDir == "" {
		return Removed{}, errors.New("no corpus directory configured: removal is disabled")
	}

	out := Removed{Path: rel}
	src := filepath.Join(e.corpusDir, filepath.FromSlash(rel))
	switch _, statErr := os.Stat(src); {
	case statErr == nil:
		dst := filepath.Join(e.corpusDir, TrashDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return Removed{}, fmt.Errorf("preparing trash for %s: %w", rel, err)
		}
		// Rename, not copy-then-delete: it is atomic within one filesystem, so there is
		// no window where the document exists twice and a concurrent ingest indexes both.
		if err := os.Rename(src, dst); err != nil {
			return Removed{}, fmt.Errorf("moving %s to %s: %w", rel, TrashDir, err)
		}
		out.Trash = filepath.ToSlash(filepath.Join(TrashDir, rel))
	case os.IsNotExist(statErr):
		// Indexed but not on disk: someone removed the file by hand. Clean the index
		// anyway rather than refusing — a row whose citation opens nothing is the failure
		// this is being asked to fix.
	default:
		return Removed{}, fmt.Errorf("reading %s: %w", rel, statErr)
	}

	existed, err := e.store.DeleteDocument(rel)
	if err != nil {
		return Removed{}, err
	}
	if !existed && out.Trash == "" {
		return Removed{}, fmt.Errorf("%s is not in the corpus", rel)
	}
	return out, nil
}
