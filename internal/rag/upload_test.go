package rag_test

import (
	"strings"
	"testing"

	"knowledge-engine/internal/rag"
)

// The corpus directory is a boundary. A browser can send any string as a file
// name, so every way out of the tree is checked here rather than trusted to the
// caller.
func TestSafePathRefusesEscapes(t *testing.T) {
	for _, in := range []string{
		"../secret.md",
		"../../etc/passwd.md",
		"business/../../x.md",
		"./../x.md",
		".hidden.md",
		"business/.git/config.md",
		"qa/ticket-1.md",  // reserved: must not impersonate a confirmed answer
		"QA/ticket-99.md", // …in any spelling
	} {
		if got, err := rag.SafePath(in); err == nil {
			t.Errorf("SafePath(%q) = %q, want an error", in, got)
		}
	}
}

// Folders are the point: they are what a reader browses and scopes a question to,
// so an import must be able to create them — and only them.
func TestSafePathKeepsTheFolders(t *testing.T) {
	for in, want := range map[string]string{
		"spec.md":                     "spec.md",
		"business/pricing.md":         "business/pricing.md",
		"business/pricing/2026.md":    "business/pricing/2026.md",
		"  engineering/runbook.txt  ": "engineering/runbook.txt",
		"/business/pricing.md":        "business/pricing.md",    // absolute is made relative
		"business//pricing.md":        "business/pricing.md",    // empty segment collapses
		`C:\docs\spec.md`:             "docs/spec.md",           // Windows path
		`business\pricing.md`:         "business/pricing.md",    // Windows separator
		"Quy trình/nghỉ phép.md":      "Quy trình/nghỉ phép.md", // Vietnamese survives
		"REPORT.MARKDOWN":             "REPORT.MARKDOWN",
	} {
		got, err := rag.SafePath(in)
		if err != nil {
			t.Errorf("SafePath(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SafePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// A tree nobody can hold in their head is not a scope. The limit is stated in the
// error, because the fix is to flatten a folder, not to guess.
func TestSafePathCapsTheDepth(t *testing.T) {
	ok := "a/b/c/deep.md" // MaxDepth = 4 segments
	if _, err := rag.SafePath(ok); err != nil {
		t.Errorf("SafePath(%q) errored: %v", ok, err)
	}
	tooDeep := "a/b/c/d/deep.md"
	err := errOf(rag.SafePath(tooDeep))
	if err == nil {
		t.Fatalf("SafePath(%q) was accepted", tooDeep)
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the depth error should state the limit, got %q", err)
	}
}

// The formats are the product's promise ("only .md and .txt"), and the rejection
// has to say what to do instead — a user holding a PDF needs the next step, not a
// restatement of the rule.
func TestSafePathRefusesOtherFormats(t *testing.T) {
	for _, in := range []string{"spec.pdf", "business/report.docx", "sheet.xlsx", "archive.zip", "noext"} {
		err := errOf(rag.SafePath(in))
		if err == nil {
			t.Errorf("SafePath(%q) was accepted", in)
			continue
		}
		if !strings.Contains(err.Error(), "markitdown") && in != "noext" {
			t.Errorf("SafePath(%q) error should point at the conversion step, got %q", in, err)
		}
	}
}

func TestSafePathRefusesEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "/", "//", "."} {
		if got, err := rag.SafePath(in); err == nil {
			t.Errorf("SafePath(%q) = %q, want an error", in, got)
		}
	}
}

func TestIsText(t *testing.T) {
	for in, want := range map[string]bool{
		"a.md": true, "a.markdown": true, "a.txt": true,
		"A.MD": true, "a.pdf": false, "a": false, "a.md.pdf": false,
	} {
		if got := rag.IsText(in); got != want {
			t.Errorf("IsText(%q) = %v, want %v", in, got, want)
		}
	}
}

func errOf(_ string, err error) error { return err }
