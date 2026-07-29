package rag

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"knowledge-engine/internal/db"
)

// Importing a document, which is now the only way one enters the system: the row *is* the
// document — body, attributes and all — and the index is built from it in the same call.
//
// What changed with the inversion: there is no file, so there is no second copy to
// reconcile, no `ingest docs` to repair a half-write, and no corpus directory that has to
// exist before a BA can save anything. What did not change is the gate: the app has no
// accounts, so an open import endpoint would let anyone who reaches the port rewrite what
// everyone else reads.

// Uploaded is what an import produced, per file.
type Uploaded struct {
	Path   string `json:"path"`   // the identity it is stored and cited under
	Chunks int    `json:"chunks"` // how many sections reached the index
}

// Attrs are the attributes a BA files a document under — everything about it that its text
// does not say. They are how a document is found again six months later, by a person rather
// than by retrieval.
//
// There is no folder field: the folder is in the path, which is also the scope prefix and
// the citation identity. Two spellings of one folder is the drift this repo deletes on sight.
type Attrs struct {
	Title       string `json:"title"`       // what to call it on screen; the file name if blank
	Alias       string `json:"alias"`       // the other names people ask for it by
	Kind        string `json:"kind"`        // spec · runbook · policy · faq — a BA's own vocabulary
	Description string `json:"description"` // one line: what is in it, and when to reach for it
}

// TextExts are the formats the engine indexes. Binary documents (PDF, DOCX) are
// converted upstream of this — see the README: keeping the parser out of the binary
// makes "the documents are messy" a one-time cleaning step, not a runtime failure.
var TextExts = []string{".md", ".markdown", ".txt"}

// IsText reports whether a filename is one the engine can index.
func IsText(name string) bool {
	return slices.Contains(TextExts, strings.ToLower(filepath.Ext(name)))
}

// MaxDepth caps how deep an imported path may nest. Folders are what a reader
// scopes a question to, and a tree nobody can hold in their head is not a scope —
// three levels is already "business / pricing / 2026".
const MaxDepth = 4

// Upload stores one document and indexes it, in one transaction's worth of intent.
//
// Folders are kept, because they are the structure a reader browses and scopes a question
// to — but every segment is sanitised, because a browser is free to submit
// "../../etc/passwd" and the path is an identity other tables point at.
//
// Re-uploading the same path updates that document in place rather than creating a second
// one — the same identity rule `ingest` uses, so the two agree — and brings back one that
// was removed, because that is what a person means by importing it again.
//
// There is no longer a state where the document is saved but not indexed: the row and its
// chunks are written after the embeddings come back, so a provider failure leaves the
// library exactly as it was.
func (e *Engine) Upload(ctx context.Context, name, content string, a Attrs) (Uploaded, error) {
	rel, err := SafePath(name)
	if err != nil {
		return Uploaded{}, err
	}
	return e.save(ctx, rel, content, a)
}

// save is the write both doors share, and the only place that decides what a stored document
// costs a caller: empty text is refused, one ingest writes the row and its chunks together,
// and nothing is reported as saved that no chunk came out of. Upload and Update held a copy
// each — two copies of "what saving means", one commit away from disagreeing.
func (e *Engine) save(ctx context.Context, rel, content string, a Attrs) (Uploaded, error) {
	if strings.TrimSpace(content) == "" {
		return Uploaded{}, fmt.Errorf("%s is empty", rel)
	}
	n, err := e.ingest(ctx, doc(rel, content, a))
	if err != nil {
		return Uploaded{Path: rel}, fmt.Errorf("%s was not saved (%w)", rel, err)
	}
	if n == 0 {
		return Uploaded{Path: rel}, fmt.Errorf("%s has no indexable text", rel)
	}
	return Uploaded{Path: rel, Chunks: n}, nil
}

// Stored is one document read back whole — the attributes and the text. It is what an edit
// form loads, and the reason the list does not carry bodies.
type Stored struct {
	Path  string `json:"path"`
	Attrs        // flattened: title · alias · kind · description
	Body  string `json:"body"`
}

// Document reads one document, body included. Reports whether it exists.
//
// A removed document still reads — the row keeps its text — which is what makes the trash
// column recoverable through the same door rather than through a second one.
func (e *Engine) Document(name string) (Stored, bool, error) {
	rel, err := readPath(name)
	if err != nil {
		return Stored{}, false, err
	}
	d, ok, err := e.store.Document(rel)
	if err != nil || !ok {
		return Stored{}, ok, err
	}
	return Stored{
		Path: d.Path,
		Attrs: Attrs{
			Title: d.Title, Alias: d.Alias, Kind: d.Kind, Description: d.Description,
		},
		Body: d.Body,
	}, true, nil
}

// Update is the edit: new text, new attributes, or a new path — one call, because a BA
// renaming a document while fixing a typo in it is one action and must not be able to half
// happen.
//
// A move is a delete-then-write rather than an UPDATE of the path, and deliberately so: the
// path is the citation identity and the scope prefix, so the chunks under the old one have
// to go with it. Re-embedding is the cost of that honesty, and it is what a rename *is* —
// the same document at another address, retrievable there and nowhere else.
//
// An empty `to` means "same path", which is the common case: it keeps the caller from having
// to echo the identity back to change a description.
func (e *Engine) Update(ctx context.Context, from, to, content string, a Attrs) (Uploaded, error) {
	// readPath here, SafePath below: correcting a confirmed answer where it already lives is
	// an edit, while moving anything *into* qa/ would be the fabrication the import rule
	// exists to refuse.
	src, err := readPath(from)
	if err != nil {
		return Uploaded{}, err
	}
	dst := src
	if strings.TrimSpace(to) != "" {
		if dst, err = SafePath(to); err != nil {
			return Uploaded{}, err
		}
	}
	saved, err := e.save(ctx, dst, content, a)
	// Only after the new address holds the document: a rename that dropped the old rows
	// first and then failed to embed would lose the document outright.
	if err != nil || dst == src {
		return saved, err
	}
	if _, err := e.store.RemoveDocument(src); err != nil {
		return saved, fmt.Errorf("%s was saved as %s but %s still answers (%w)", src, dst, src, err)
	}
	return saved, nil
}

// doc is where an empty title becomes the file name, in one place, so a document imported
// without one is not called "" on every screen that lists it.
func doc(rel, content string, a Attrs) db.Doc {
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	return db.Doc{
		Path: rel, Title: title, Alias: strings.TrimSpace(a.Alias),
		Kind: strings.TrimSpace(a.Kind), Description: strings.TrimSpace(a.Description),
		Body: content,
	}
}

// segments splits a cleaned path and keeps only the parts that may become one.
//
// Extracted because the qa/ flag pushed safePath over gocyclo's 16, and a limit raised to
// fit a function is a limit that no longer means anything. This is the half that answers one
// question — "is this segment allowed?" — so it reads on its own, and `name` travels with it
// only to name the offender in the error the person sees.
func segments(clean, name string) ([]string, error) {
	var segs []string
	for s := range strings.SplitSeq(clean, "/") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue // leading "/", "//", trailing "/"
		}
		if s == "." || s == ".." {
			return nil, fmt.Errorf("%q walks outside the documents folder", name)
		}
		if strings.HasPrefix(s, ".") {
			return nil, fmt.Errorf("%q has a hidden segment (%s)", name, s)
		}
		// A path is read by people in citations and in a document list; a control character
		// makes both unreadable. Vietnamese and spaces are fine.
		for _, r := range s {
			if unicode.IsControl(r) {
				return nil, fmt.Errorf("%q contains a control character", name)
			}
		}
		segs = append(segs, s)
	}
	return segs, nil
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
func SafePath(name string) (string, error) { return safePath(name, false) }

// readPath is SafePath for a path that already exists rather than one being created: the
// structural rules are identical, and the qa/ refusal is not.
//
// That refusal stops an *import* from fabricating an answer a BA never vouched for. Reading
// one back, or fixing a typo in one, is not that — and a BA who cannot correct a published
// answer would have to file a second ticket about their own.
func readPath(name string) (string, error) { return safePath(name, true) }

func safePath(name string, allowQA bool) (string, error) {
	// Windows separators first: under Linux filepath treats "docs\spec.md" as one
	// file name, and the backslashes would end up in the stored path.
	clean := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	// A drive letter or UNC prefix is absolute on the machine it came from; keep
	// only what follows so it lands in the corpus like any other path.
	if i := strings.Index(clean, ":"); i >= 0 && i <= 2 {
		clean = clean[i+1:]
	}

	segs, err := segments(clean, name)
	if err != nil {
		return "", err
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
	if !allowQA && strings.EqualFold(segs[0], QADir) {
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
