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
	lines := strings.Split(clean(md), "\n")
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

// ── the noise a document arrives with ─────────────────────────────────────────
//
// A document is written in a browser or pasted out of Confluence, Slack or Word, and what
// comes with it is invisible: a BOM, non-breaking spaces where spaces belong, zero-width
// joiners, CRLF, an HTML comment nobody meant to keep, a front-matter block from whatever
// exported it. None of it shows in the form and all of it reaches the embedder and the BM25
// index — a no-break space makes "hoàn tiền" a token no query will match, and a section
// opening with `<!-- generated -->` spends part of a chunk saying nothing.
//
// Two rules make this safe to do at all:
//
//  1. The body is never touched. This runs on the way to `chunks`, which are derived and
//     rebuilt by any re-index. `documents.body` keeps exactly what the BA saved, because
//     that is the text a person vouched for (invariant 1) and the text they see again on
//     opening it. Cleaning it there would mean the library shows something nobody typed.
//  2. A fenced block is left alone, apart from line endings. Trailing spaces are significant
//     in code, a no-break space inside a snippet may be the very thing a reader is being
//     warned about, and `<!-- -->` in a fence is an example rather than a comment. Whatever
//     the document put in a fence, it meant.
//
// Everything already indexed keeps the chunks it has until it is written again or
// re-ingested: this changes what indexing does, not what is already stored.
func clean(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")
	md = strings.ReplaceAll(md, "\ufeff", "") // a BOM survives a copy-paste and is not a word

	var (
		b      strings.Builder
		fenced bool
		blanks int
	)
	for line := range strings.SplitSeq(stripFrontMatter(md), "\n") {
		if isFence(line) {
			fenced = !fenced
		}
		if !fenced && !isFence(line) {
			line = stripComments(invisible.Replace(line))
			line = strings.TrimRight(line, " \t")
			// Three blank lines and thirty read the same to a chunker that splits on one, and
			// the difference is chunk budget spent on nothing.
			if line == "" {
				if blanks++; blanks > 1 {
					continue
				}
			} else {
				blanks = 0
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// invisible maps the characters that look like a space and are not one. A non-breaking space
// makes "hoàn tiền" a different token to every retriever this engine has.
var invisible = strings.NewReplacer(
	"\u00a0", " ", // no-break space
	"\u202f", " ", // narrow no-break space
	"\u2007", " ", // figure space
	"\u200b", "", // zero-width space
	"\u200c", "", // zero-width non-joiner
	"\u200d", "", // zero-width joiner
	"\u2060", "", // word joiner
)

func isFence(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// stripFrontMatter drops a leading `---` block. Exporters write one and it is metadata about
// the document rather than anything the document says, so it competes for a chunk and answers
// nothing. Only at the very start, and only when it closes — a document that opens with a
// horizontal rule keeps it.
func stripFrontMatter(md string) string {
	if !strings.HasPrefix(md, "---\n") {
		return md
	}
	if _, rest, ok := strings.Cut(md[4:], "\n---\n"); ok {
		return rest
	}
	return md
}

// stripComments removes single-line HTML comments. Deliberately not multi-line: a comment
// that opens on one line and closes on another would need the state machine above to carry a
// second mode, and the case that turns up in a pasted document is `<!-- -->` on its own line.
func stripComments(line string) string {
	for {
		open := strings.Index(line, "<!--")
		if open < 0 {
			return line
		}
		shut := strings.Index(line[open:], "-->")
		if shut < 0 {
			return line
		}
		line = line[:open] + line[open+shut+3:]
	}
}
