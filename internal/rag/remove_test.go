package rag_test

import (
	"context"
	"strings"
	"testing"

	"knowledge-engine/internal/rag"
)

// The property that makes this a *soft* delete, and the reason it still is one after the
// inversion: the database holds the documents now, so a hard delete would be the only
// operation in this system that destroys an original — including a BA-confirmed answer,
// which a person wrote and nothing can rebuild.
//
// What it asserts, in the order that matters: the document stops answering, its text
// survives, and importing it again brings it back rather than leaving a row that is both
// present and removed.
func TestARemovedDocumentStopsAnsweringAndItsTextSurvives(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()

	const rel, body = "booking/pricing.md", "# Pricing\n\nthe rule a BA confirmed\n"
	if _, err := e.Upload(ctx, rel, body, rag.Attrs{Title: "Pricing", Kind: "policy"}); err != nil {
		t.Fatalf("upload: %v", err)
	}

	if _, err := e.Remove(ctx, rel); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// 1. It is out of the library, so nothing lists it and no scope reaches it.
	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range c.Documents {
		if d.Path == rel {
			t.Error("a removed document is still listed in the corpus")
		}
	}

	// 2. Its text is still there — the trash, as a column. This is what recovery means now
	//    that no directory holds a second copy.
	doc, ok, err := e.Document(rel)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !strings.Contains(doc.Body, "the rule a BA confirmed") {
		t.Errorf("the bytes were destroyed rather than kept (ok=%v): %q", ok, doc.Body)
	}

	// 3. Removing it twice says so rather than reporting a second success: a client that
	//    cannot tell "gone" from "was never here" shows the wrong thing to whoever clicked.
	if _, err := e.Remove(ctx, rel); err == nil {
		t.Error("removing an already-removed document was reported as a success")
	}

	// 4. Importing it again is the way back, and it leaves no row that is both present and
	//    deleted — the state that would list a document retrieval cannot return.
	if _, err := e.Upload(ctx, rel, body, rag.Attrs{Title: "Pricing"}); err != nil {
		t.Fatalf("re-import after removal: %v", err)
	}
	c, err = e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	back := false
	for _, d := range c.Documents {
		if d.Path == rel {
			back = true
		}
	}
	if !back {
		t.Error("importing a removed document again did not bring it back")
	}
}

// A rename is the same document at another address, and it must be retrievable there and
// nowhere else. The failure this guards is the half-move: the new path saved, the old one
// still answering, so one document cites two sources and a scope matches both.
func TestRenamingADocumentLeavesNothingBehind(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()

	const from, to = "drafts/spec.md", "specs/billing.md"
	if _, err := e.Upload(ctx, from, "# Billing\n\ninvoices are void after 30 days\n", rag.Attrs{}); err != nil {
		t.Fatalf("upload: %v", err)
	}

	saved, err := e.Update(ctx, from, to, "# Billing\n\ninvoices are void after 30 days\n",
		rag.Attrs{Title: "Billing", Alias: "invoice rules", Kind: "spec", Description: "when an invoice expires"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if saved.Path != to {
		t.Errorf("saved as %q, want %q", saved.Path, to)
	}

	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	var found *string
	for i, d := range c.Documents {
		if d.Path == from {
			t.Error("the old path is still in the library — a rename that answers twice")
		}
		if d.Path == to {
			found = &c.Documents[i].Alias
		}
	}
	if found == nil {
		t.Fatal("the renamed document is not in the library")
	}
	// The attributes travelled with it. A rename that drops what a BA typed is a rename
	// nobody will use twice.
	if *found != "invoice rules" {
		t.Errorf("alias after rename = %q, want it carried over", *found)
	}
}
