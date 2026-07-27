package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Importing a document, the same way a confirmed answer arrives: a file in the
// corpus directory, then indexed. Nothing is stored that `ingest docs` could not
// rebuild — the corpus stays the source of truth, and the database stays derived.
//
// This is deliberately the *only* write path besides Confirm, and it takes the same
// gate. The app has no accounts, so an open import endpoint would let anyone who
// reaches the port rewrite what everyone else reads.

// Uploaded is what an import produced, per file.
type Uploaded struct {
	Path   string `json:"path"`   // the identity it is stored and cited under
	Chunks int    `json:"chunks"` // how many sections reached the index
}

// TextExts are the formats the engine indexes. Binary documents (PDF, DOCX) are
// converted upstream of this — see the README: keeping the parser out of the binary
// makes "the documents are messy" a one-time cleaning step, not a runtime failure.
var TextExts = []string{".md", ".markdown", ".txt"}

// IsText reports whether a filename is one the engine can index.
func IsText(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range TextExts {
		if ext == e {
			return true
		}
	}
	return false
}

// Upload writes one document into the corpus and indexes it.
//
// The stored path is derived from the file name alone, never from what the client
// sent as a path: a browser is free to submit "../../etc/passwd" or an absolute
// path, and the corpus directory is a boundary, not a suggestion.
//
// Re-uploading the same name updates that document in place rather than creating a
// second one — the same identity rule ingest uses, so the two agree.
func (e *Engine) Upload(ctx context.Context, name string, content string) (Uploaded, error) {
	rel, err := SafeName(name)
	if err != nil {
		return Uploaded{}, err
	}
	if strings.TrimSpace(content) == "" {
		return Uploaded{}, fmt.Errorf("%s is empty", rel)
	}
	if err := e.writeDoc(rel, content); err != nil {
		return Uploaded{}, err
	}
	n, err := e.Ingest(ctx, rel, content)
	if err != nil {
		// The file is on disk and is the source of truth, so say what recovers it
		// rather than pretending nothing happened.
		return Uploaded{Path: rel}, fmt.Errorf("%s was saved but not indexed (%w) — `ingest docs` will pick it up", rel, err)
	}
	if n == 0 {
		return Uploaded{Path: rel}, fmt.Errorf("%s has no indexable text", rel)
	}
	return Uploaded{Path: rel, Chunks: n}, nil
}

// SafeName reduces whatever a client sent to a plain file name inside the corpus.
//
// Only the base name survives, so no input can traverse out of the tree or land in
// qa/ and impersonate a confirmed answer. What remains is checked for the one thing
// a name still has to be: a text document this engine can read.
func SafeName(name string) (string, error) {
	// Windows separators too: filepath.Base is a no-op on "C:\docs\spec.md" under
	// Linux, and the whole string would become one file name.
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(filepath.Base(name))

	// A leading dot is how a name becomes invisible in the corpus folder, and "."
	// / ".." survive Base unchanged.
	if name == "" || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("%q is not a usable file name", name)
	}
	if !IsText(name) {
		return "", fmt.Errorf("%s is not a %s file — convert it first (markitdown spec.pdf > spec.md)",
			name, strings.Join(TextExts, " / "))
	}
	// Control characters would make the citation line unreadable and the file
	// awkward to open by hand; everything else, including Vietnamese, is fine.
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%q contains a control character", name)
		}
	}
	return name, nil
}
