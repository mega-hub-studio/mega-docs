package rag

import (
	"strings"
	"testing"
)

// A specification is mostly short numbered sub-headings, and one of those on its own
// is not a retrievable unit: it competes for a TOP_K slot it cannot fill. These tests
// pin the merge rule, because the failure it prevents is invisible — answers get
// thinner, nothing errors.
func TestUndersizedSectionsMergeUntilTheyAreWorthRetrieving(t *testing.T) {
	md := `# Waiting list

## 6.1 Grid view
Shows one row per request.

## 6.2 Behavior
Sorted by deposit date.

## 6.3 Empty state
A dashed panel.
`
	chunks := SplitMarkdown(md)
	if len(chunks) != 1 {
		t.Fatalf("three tiny sections became %d chunks; want 1:\n%+v", len(chunks), chunks)
	}
	c := chunks[0]
	// Each section's own heading has to survive into the text, or a merged chunk can
	// no longer say which sub-section a sentence came from.
	for _, want := range []string{"6.1 Grid view", "6.2 Behavior", "6.3 Empty state", "dashed panel"} {
		if !strings.Contains(c.Content, want) {
			t.Errorf("merged chunk lost %q:\n%s", want, c.Content)
		}
	}
	// The breadcrumb is what a citation shows, so it must be the heading the merged
	// sections actually share — never one of the three, which would misattribute the
	// other two.
	if c.Heading != "Waiting list" {
		t.Errorf("heading = %q; want the shared ancestor %q", c.Heading, "Waiting list")
	}
}

func TestASectionBigEnoughToStandAloneIsLeftAlone(t *testing.T) {
	big := strings.Repeat("A deposit is refundable within seven days. ", 20) // > minChars
	md := "# Rules\n\n## Deposits\n" + big + "\n\n## Note\nShort.\n"

	chunks := SplitMarkdown(md)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks; want the big section alone plus the short one", len(chunks))
	}
	if chunks[0].Heading != "Rules > Deposits" {
		t.Errorf("a standalone section lost its own heading: %q", chunks[0].Heading)
	}
	if !strings.Contains(chunks[1].Content, "Short.") {
		t.Errorf("the trailing short section was dropped: %+v", chunks[1])
	}
}

// Merging must never undo the maxChars guarantee the embedding call depends on.
func TestNoChunkExceedsMaxChars(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Doc\n")
	for range 200 {
		b.WriteString("\n## Section\n")
		b.WriteString(strings.Repeat("x", 400))
		b.WriteString("\n")
	}
	for i, c := range SplitMarkdown(b.String()) {
		if len(c.Content) > maxChars {
			t.Fatalf("chunk %d is %d chars, over the %d limit", i, len(c.Content), maxChars)
		}
	}
}

// An oversized section still splits by paragraph, and the parts keep their own
// heading rather than inheriting a merged one.
func TestOversizedSectionsStillSplit(t *testing.T) {
	para := strings.Repeat("word ", 200) + "\n\n"
	md := "# Doc\n\n## Long\n" + strings.Repeat(para, 6)

	chunks := SplitMarkdown(md)
	if len(chunks) < 2 {
		t.Fatalf("a %d-char section produced %d chunk(s)", len(md), len(chunks))
	}
	for _, c := range chunks {
		if c.Heading != "Doc > Long" {
			t.Errorf("split part has heading %q; want the section's own", c.Heading)
		}
	}
}

func TestSharedCrumb(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{"Doc > 6. UI > 6.1 Grid", "Doc > 6. UI > 6.2 Rows", "Doc > 6. UI"},
		{"Doc > 6. UI", "Doc > 7. Data", "Doc"},
		{"Alpha", "Beta", "Alpha"}, // nothing shared: keep one rather than none
		{"Doc > A", "Doc > A", "Doc > A"},
	} {
		if got := sharedCrumb(c.a, c.b); got != c.want {
			t.Errorf("sharedCrumb(%q, %q) = %q; want %q", c.a, c.b, got, c.want)
		}
	}
}

// A markdown table is one paragraph with no blank lines in it, so paragraph packing
// alone left whole business-rules tables as single 11k-character chunks — measured on
// a real corpus, ten of them. Each part must stay a *table*: header row and separator
// repeated, or the rows in every part after the first have unnamed columns.
func TestALongTableIsSplitAndEveryPartKeepsItsHeader(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Rules\n\n## Business rules\n")
	b.WriteString("| Code | Rule | Handling |\n|---|---|---|\n")
	for range 120 {
		b.WriteString("| BR-")
		b.WriteString(strings.Repeat("0", 2))
		b.WriteString(" | a rule that is long enough to matter | handled by the server |\n")
	}

	chunks := SplitMarkdown(b.String())
	if len(chunks) < 2 {
		t.Fatalf("a %d-char table produced %d chunk(s)", b.Len(), len(chunks))
	}
	for i, c := range chunks {
		if len(c.Content) > maxChars {
			t.Errorf("part %d is %d chars, over the %d limit", i, len(c.Content), maxChars)
		}
		if !strings.Contains(c.Content, "| Code | Rule | Handling |") {
			t.Errorf("part %d lost the table header:\n%s", i, first(c.Content, 120))
		}
		if !strings.Contains(c.Content, "|---|---|---|") {
			t.Errorf("part %d lost the separator row, so it is no longer a table", i)
		}
	}
}

func first(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// What a document arrives carrying, and what the index must not carry with it. Each pair is
// a real paste: a BOM from an export, a no-break space from Word, an exporter's front matter,
// an HTML comment from a Confluence page, and Windows line endings.
//
// The no-break space is the one that costs an answer rather than a few bytes: it makes
// "hoàn tiền" a token no query produces, so a document that plainly answers the question
// stops being retrievable by the words it is written in.
func TestIndexingDropsTheNoiseADocumentArrivedWith(t *testing.T) {
	md := "\ufeff---\ntitle: exported\nauthor: nobody\n---\n" +
		"# Refund\n\n<!-- generated by the exporter -->\n" +
		"A refund is issued\u00a0within 24h.   \r\n\n\n\n" +
		"Zero\u200bwidth joins are gone too.\n"

	got := SplitMarkdown(md)
	if len(got) == 0 {
		t.Fatal("nothing survived the clean")
	}
	body := got[0].Content
	for _, bad := range []string{"\ufeff", "\u00a0", "\u200b", "\r", "<!--", "title: exported"} {
		if strings.Contains(body, bad) {
			t.Errorf("%q reached the index:\n%q", bad, body)
		}
	}
	if !strings.Contains(body, "hoàn tiền was here") && !strings.Contains(body, "refund is issued within 24h") {
		t.Errorf("the no-break space was removed instead of turned back into a space:\n%q", body)
	}
	if strings.Contains(body, "\n\n\n") {
		t.Errorf("a run of blank lines survived:\n%q", body)
	}
}

// The other half, and the one that decides whether this is a cleaner or a corruption: inside
// a fence the document meant exactly what it wrote. A trailing space can be the subject of
// the snippet, a no-break space can be the bug being demonstrated, and `<!-- -->` is an
// example of a comment rather than one.
func TestAFencedBlockKeepsWhatItWasGiven(t *testing.T) {
	md := "# Code\n\n" +
		"Some prose\u00a0here.\n\n" +
		"```html\n<!-- this comment is the example -->\nconst a = 1;   \nnbsp:\u00a0here\n```\n"

	got := SplitMarkdown(md)
	if len(got) == 0 {
		t.Fatal("nothing came back")
	}
	body := got[0].Content
	for _, kept := range []string{"<!-- this comment is the example -->", "const a = 1;   ", "nbsp:\u00a0here"} {
		if !strings.Contains(body, kept) {
			t.Errorf("the fence lost %q:\n%q", kept, body)
		}
	}
	if strings.Contains(body, "prose\u00a0here") {
		t.Errorf("prose outside the fence was left dirty:\n%q", body)
	}
}
