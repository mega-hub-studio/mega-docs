package rag

import (
	"context"
	"fmt"
	"path"
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

// MaxDepth caps how deep an imported path may nest. Folders are what a reader
// scopes a question to, and a tree nobody can hold in their head is not a scope —
// three levels is already "business / pricing / 2026".
const MaxDepth = 4

// Upload writes one document into the corpus and indexes it.
//
// Folders are kept, because they are the structure a reader browses and scopes a
// question to — but every segment is sanitised, and the corpus directory is a
// boundary, not a suggestion: a browser is free to submit "../../etc/passwd".
//
// Re-uploading the same path updates that document in place rather than creating a
// second one — the same identity rule ingest uses, so the two agree.
func (e *Engine) Upload(ctx context.Context, name string, content string) (Uploaded, error) {
	rel, err := SafePath(name)
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

// SafePath turns whatever a client sent into a relative path inside the corpus.
//
// Folders survive — "business/pricing/2026.md" stays that — because the tree is
// what a reader browses and scopes a question to. Everything that could take the
// path somewhere else does not:
//
//   - a leading "/" or a drive letter, which would make it absolute
//   - any ".." segment, the way out of the tree
//   - a "." segment or a dot-prefixed name, which hides a file from the folder
//     it is supposed to appear in
//   - the reserved qa/ folder, so an import cannot impersonate an answer a BA
//     vouched for (those carry an approval boost in retrieval)
//
// The check is per segment and structural, not a blocklist of strings: it is the
// resulting path that has to be inside the tree, whatever spelling produced it.
func SafePath(name string) (string, error) {
	// Windows separators first: under Linux filepath treats "docs\spec.md" as one
	// file name, and the backslashes would end up in the stored path.
	clean := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	// A drive letter or UNC prefix is absolute on the machine it came from; keep
	// only what follows so it lands in the corpus like any other path.
	if i := strings.Index(clean, ":"); i >= 0 && i <= 2 {
		clean = clean[i+1:]
	}

	var segs []string
	for _, s := range strings.Split(clean, "/") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue // leading "/", "//", trailing "/"
		}
		if s == "." || s == ".." {
			return "", fmt.Errorf("%q walks outside the documents folder", name)
		}
		if strings.HasPrefix(s, ".") {
			return "", fmt.Errorf("%q has a hidden segment (%s)", name, s)
		}
		// A path is read by people in citations and in a file browser; a control
		// character makes both unreadable. Vietnamese and spaces are fine.
		for _, r := range s {
			if unicode.IsControl(r) {
				return "", fmt.Errorf("%q contains a control character", name)
			}
		}
		segs = append(segs, s)
	}
	if len(segs) == 0 {
		return "", fmt.Errorf("%q is not a usable file name", name)
	}
	if len(segs) > MaxDepth {
		return "", fmt.Errorf("%s is %d folders deep — the limit is %d", clean, len(segs)-1, MaxDepth-1)
	}
	if !IsText(segs[len(segs)-1]) {
		return "", fmt.Errorf("%s is not a %s file — convert it first (markitdown spec.pdf > spec.md)",
			segs[len(segs)-1], strings.Join(TextExts, " / "))
	}
	if strings.EqualFold(segs[0], QADir) {
		return "", fmt.Errorf("%s/ holds answers a BA confirmed — import into another folder", QADir)
	}

	rel := path.Join(segs...)
	// Belt and braces: whatever the segment rules missed, the result must still be
	// a relative path that stays put.
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%q walks outside the documents folder", name)
	}
	return rel, nil
}
