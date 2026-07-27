package rag_test

import (
	"strings"
	"testing"

	"knowledge-engine/internal/rag"
)

// The corpus directory is a boundary. A browser can send any string as a file
// name, so every way out of the tree is checked here rather than trusted to the
// caller.
func TestSafeNameRefusesEscapes(t *testing.T) {
	for _, in := range []string{
		"../secret.md",
		"../../etc/passwd.md",
		"/etc/passwd.md",
		`C:\Windows\notes.md`,
		"docs/../../x.md",
		"qa/ticket-1.md", // must not be able to impersonate a confirmed answer
	} {
		got, err := rag.SafeName(in)
		if err != nil {
			continue // refused outright is fine
		}
		if strings.ContainsAny(got, `/\`) || strings.Contains(got, "..") {
			t.Errorf("SafeName(%q) = %q — still a path", in, got)
		}
	}
}

func TestSafeNameKeepsThePlainName(t *testing.T) {
	for in, want := range map[string]string{
		"spec.md":                     "spec.md",
		"  onboarding.txt  ":          "onboarding.txt",
		"docs/api/spec.md":            "spec.md",
		"Quy trình nghỉ phép.md":      "Quy trình nghỉ phép.md", // Vietnamese survives
		"REPORT.MARKDOWN":             "REPORT.MARKDOWN",
		`\\server\share\handbook.txt`: "handbook.txt",
	} {
		got, err := rag.SafeName(in)
		if err != nil {
			t.Errorf("SafeName(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The formats are the product's promise ("only .md and .txt"), and the rejection
// has to say what to do instead — a user holding a PDF needs the next step, not a
// restatement of the rule.
func TestSafeNameRefusesOtherFormats(t *testing.T) {
	for _, in := range []string{"spec.pdf", "report.docx", "sheet.xlsx", "archive.zip", "noext"} {
		_, err := rag.SafeName(in)
		if err == nil {
			t.Errorf("SafeName(%q) was accepted", in)
			continue
		}
		if !strings.Contains(err.Error(), "markitdown") && in != "noext" {
			t.Errorf("SafeName(%q) error should point at the conversion step, got %q", in, err)
		}
	}
}

func TestSafeNameRefusesHiddenAndEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", ".", "..", ".env", ".hidden.md"} {
		if got, err := rag.SafeName(in); err == nil {
			t.Errorf("SafeName(%q) = %q, want an error", in, got)
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
