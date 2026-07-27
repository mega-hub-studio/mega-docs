package main

import (
	"path/filepath"
	"testing"
)

// The stored document path is an identity: it is what a citation shows, and what a
// re-ingest matches on. Every row here is a way the same file could otherwise end up
// stored twice — once by an operator's `ingest` and once by a BA's confirm.
func TestDocPathIsTheSameHoweverIngestWasInvoked(t *testing.T) {
	abs := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	cases := []struct {
		name, corpus, file, want string
	}{
		{"relative corpus, relative file", "docs", "docs/spec.md", "spec.md"},
		{"relative corpus, absolute file", "docs", abs("docs/spec.md"), "spec.md"},
		{"absolute corpus, relative file", abs("docs"), "docs/spec.md", "spec.md"},
		{"nested", "docs", "docs/api/v2/spec.md", "api/v2/spec.md"},
		{"a confirmed answer", "docs", "docs/qa/ticket-7.md", "qa/ticket-7.md"},
		{"trailing slash on the corpus", "docs/", "docs/spec.md", "spec.md"},
		{"dot segments", "docs", "docs/./api/../spec.md", "spec.md"},

		// Outside the corpus there is nothing honest to shorten to, and inventing a
		// relative path would collide with a real corpus entry.
		{"sibling directory", "docs", "../elsewhere/spec.md", "../elsewhere/spec.md"},
		{"no corpus configured", "", "docs/spec.md", "docs/spec.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := docPath(c.corpus, c.file); got != c.want {
				t.Errorf("docPath(%q, %q) = %q, want %q", c.corpus, c.file, got, c.want)
			}
		})
	}
}

func TestOnlyTextDocumentsAreIngested(t *testing.T) {
	for _, p := range []string{"a.md", "A.MD", "notes.txt", "readme.markdown"} {
		if !isDoc(p) {
			t.Errorf("%s should be ingested", p)
		}
	}
	// Binary formats are converted to markdown first, on purpose — see the README.
	for _, p := range []string{"spec.pdf", "deck.pptx", "sheet.xlsx", "image.png", "notes"} {
		if isDoc(p) {
			t.Errorf("%s should not be ingested as text", p)
		}
	}
}
