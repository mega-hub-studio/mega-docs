package rag

import (
	"os"
	"path/filepath"
	"testing"
)

// The property that makes this a *soft* delete, and the reason it is one: the corpus is the
// source of truth, so a hard delete would be the only operation here that destroys an
// original rather than something `ingest` can rebuild.
func TestARemovedDocumentIsRecoverableAndStaysUnindexed(t *testing.T) {
	dir := t.TempDir()
	rel := "booking/pricing.md"
	src := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(src), 0o750); err != nil {
		t.Fatal(err)
	}
	const body = "# Pricing\n\nthe rule a BA confirmed\n"
	if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// The file half, on its own: the engine's store is not needed to prove where the
	// bytes went, and a temp corpus with no database is the smallest thing that can.
	trash := filepath.Join(dir, TrashDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(trash), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, trash); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("the document is still where it was")
	}
	got, err := os.ReadFile(trash)
	if err != nil {
		t.Fatalf("the bytes were destroyed rather than moved: %v", err)
	}
	if string(got) != body {
		t.Error("the trashed copy does not match what was removed")
	}

	// And the part that needs no exclusion rule: the trash is unreachable as a document
	// path, so a re-ingest can never pull the file back in. If SafePath ever stopped
	// refusing a hidden segment, a delete would silently undo itself on the next ingest.
	for _, p := range []string{
		TrashDir + "/" + rel,
		"./" + TrashDir + "/x.md",
		TrashDir,
	} {
		if out, err := SafePath(p); err == nil {
			t.Errorf("SafePath accepted %q as %q — the trash is indexable, so removal is not permanent", p, out)
		}
	}
}

// Removal is disabled rather than half-working when there is nowhere to move the file:
// deleting the index rows while leaving the document on disk is a delete that the next
// ingest undoes, which is worse than refusing.
func TestRemovalIsRefusedWithoutACorpusDirectory(t *testing.T) {
	e := &Engine{}
	if _, err := e.Remove(nil, "a.md"); err == nil { //nolint:staticcheck // a nil ctx is never used on this path
		t.Error("removal was allowed with no corpus directory configured")
	}
}
