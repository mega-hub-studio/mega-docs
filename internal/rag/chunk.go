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
	overlap  = 250  // chars carried into the next chunk for context continuity
)

// SplitMarkdown does structural, heading-aware chunking.
// It never crosses a heading boundary and splits long sections by paragraph.
func SplitMarkdown(md string) []Chunk {
	lines := strings.Split(md, "\n")
	var (
		out     []Chunk
		crumb   []string // heading stack -> breadcrumb
		buf     strings.Builder
		curHead string
	)

	flush := func() {
		body := strings.TrimSpace(buf.String())
		buf.Reset()
		if body == "" {
			return
		}
		// Split oversized bodies by paragraph, with overlap.
		for _, part := range packParagraphs(body) {
			out = append(out, Chunk{Heading: curHead, Content: part})
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
			curHead = strings.Join(nonEmpty(crumb), " > ")
			continue
		}
		buf.WriteString(ln)
		buf.WriteString("\n")
	}
	flush()
	return out
}

func packParagraphs(body string) []string {
	if len(body) <= maxChars {
		return []string{body}
	}
	paras := strings.Split(body, "\n\n")
	var parts []string
	var cur strings.Builder
	for _, p := range paras {
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
