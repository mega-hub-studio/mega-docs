package rag

import (
	"strings"
)

// Chunk is a retrievable unit with its heading breadcrumb.
type Chunk struct {
	Heading string
	Content string
}

const (
	maxChars = 2400 // ~600 tokens
	// minChars is the size below which a section cannot be retrieved on its own
	// merit, so it is merged with the sections next to it.
	//
	// Measured, not guessed: a real corpus of five specification documents produced
	// 471 chunks with a median of 315 characters, 228 of them under 300 and the
	// smallest 19 — because the chunker split oversized sections but never joined
	// undersized ones, and a spec is mostly short numbered sub-headings. Six such
	// chunks are about 500 tokens of context for a whole answer, and a 19-character
	// chunk is noise competing for one of the six slots.
	minChars = 600
	overlap  = 250 // chars carried into the next chunk for context continuity
)

// crumbSep joins the heading stack into the breadcrumb a citation shows.
const crumbSep = " > "

// SplitMarkdown does structural, heading-aware chunking: it never merges across a
// document, splits oversized sections by paragraph, and joins consecutive undersized
// ones until they are worth retrieving.
//
// A merged chunk keeps both halves of its provenance: the breadcrumb becomes the
// deepest heading the merged sections share, and each section's own heading stays
// inline in the text, so the model still sees which sub-section said what and the
// citation still names something a reader can find.
func SplitMarkdown(md string) []Chunk {
	lines := strings.Split(md, "\n")
	var (
		out     []Chunk
		pend    *Chunk   // an undersized section waiting for the next one
		crumb   []string // heading stack -> breadcrumb
		buf     strings.Builder
		curHead string
	)

	emit := func() {
		if pend != nil {
			out = append(out, *pend)
			pend = nil
		}
	}

	flush := func() {
		body := strings.TrimSpace(buf.String())
		buf.Reset()
		if body == "" {
			return
		}
		switch {
		case len(body) > maxChars:
			// Oversized: paragraph-split, and every part stands on its own.
			emit()
			for _, part := range packParagraphs(body) {
				out = append(out, Chunk{Heading: curHead, Content: part})
			}
		case len(body) >= minChars:
			emit()
			out = append(out, Chunk{Heading: curHead, Content: body})
		case pend == nil:
			// The heading goes inline from the start: once this chunk merges, its own
			// breadcrumb is replaced by the shared one, and without the inline copy
			// the first section is the one that loses its name.
			pend = &Chunk{Heading: curHead, Content: subHeading(curHead) + "\n" + body}
		case len(pend.Content)+len(body) <= maxChars:
			pend.Content += "\n\n" + subHeading(curHead) + "\n" + body
			pend.Heading = sharedCrumb(pend.Heading, curHead)
		default:
			emit()
			pend = &Chunk{Heading: curHead, Content: subHeading(curHead) + "\n" + body}
		}
		// Stop merging as soon as it is retrievable. Filling all the way to maxChars
		// would trade the precision of a small section for the recall of a big one,
		// and the point here is only to get off the floor.
		if pend != nil && len(pend.Content) >= minChars {
			emit()
		}
	}

	for _, ln := range lines {
		if lvl, text := headingOf(ln); lvl > 0 {
			flush()
			// Update breadcrumb stack to current depth.
			if lvl-1 < len(crumb) {
				crumb = crumb[:lvl-1]
			}
			for len(crumb) < lvl-1 {
				crumb = append(crumb, "")
			}
			crumb = append(crumb, text)
			curHead = strings.Join(nonEmpty(crumb), crumbSep)
			continue
		}
		buf.WriteString(ln)
		buf.WriteString("\n")
	}
	flush()
	emit()
	return out
}

// subHeading renders the section's own heading back into the text of a merged chunk.
// Markdown, because the surrounding text is markdown and the model reads it as the
// structure it is.
func subHeading(crumb string) string {
	parts := strings.Split(crumb, crumbSep)
	return "## " + parts[len(parts)-1]
}

// sharedCrumb is the deepest breadcrumb two merged sections have in common — their
// nearest common ancestor. With nothing in common (two top-level sections) the first
// one is kept: a chunk with no heading at all would cite nothing.
func sharedCrumb(a, b string) string {
	as, bs := strings.Split(a, crumbSep), strings.Split(b, crumbSep)
	var shared []string
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			break
		}
		shared = append(shared, as[i])
	}
	if len(shared) == 0 {
		return a
	}
	return strings.Join(shared, crumbSep)
}

func packParagraphs(body string) []string {
	if len(body) <= maxChars {
		return []string{body}
	}
	paras := strings.Split(body, "\n\n")
	var parts []string
	var cur strings.Builder
	for _, p := range paras {
		// A "paragraph" bigger than the whole limit cannot be packed, only broken.
		// This is not hypothetical: a markdown table has no blank lines, so a
		// business-rules table is one paragraph — the five specification documents
		// measured here produced ten chunks over the limit and every one was a table,
		// the largest 11,651 characters. One of those fills most of a TOP_K answer
		// with rules nobody asked about.
		if len(p) > maxChars {
			if s := strings.TrimSpace(cur.String()); s != "" {
				parts = append(parts, s)
				cur.Reset()
			}
			parts = append(parts, splitLines(p)...)
			continue
		}
		if cur.Len()+len(p) > maxChars && cur.Len() > 0 {
			parts = append(parts, strings.TrimSpace(cur.String()))
			// carry overlap tail into the next part
			tail := cur.String()
			if len(tail) > overlap {
				tail = tail[len(tail)-overlap:]
			}
			cur.Reset()
			cur.WriteString(tail)
			cur.WriteString("\n\n")
		}
		cur.WriteString(p)
		cur.WriteString("\n\n")
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}

// splitLines breaks one oversized paragraph on line boundaries.
//
// A markdown table keeps its header row and separator in every part. Without them the
// second part onwards is a grid of values whose columns have no names — retrievable,
// and useless to a model asked to read a rule out of it.
func splitLines(p string) []string {
	lines := strings.Split(strings.TrimSpace(p), "\n")
	var head []string
	if len(lines) > 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "|") {
		head = lines[:2]
		lines = lines[2:]
	}

	var parts []string
	cur := append([]string{}, head...)
	for _, ln := range lines {
		grown := len(strings.Join(cur, "\n")) + 1 + len(ln)
		if grown > maxChars && len(cur) > len(head) {
			parts = append(parts, strings.Join(cur, "\n"))
			cur = append(append([]string{}, head...), ln)
			continue
		}
		cur = append(cur, ln)
	}
	if len(cur) > len(head) {
		parts = append(parts, strings.Join(cur, "\n"))
	}
	return parts
}

func headingOf(line string) (int, string) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "#") {
		return 0, ""
	}
	i := 0
	for i < len(t) && t[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(t) || t[i] != ' ' {
		return 0, ""
	}
	return i, strings.TrimSpace(t[i:])
}

func nonEmpty(in []string) []string {
	out := in[:0:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
