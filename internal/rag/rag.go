package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/db"
)

type Engine struct {
	store *db.Store
	ai    *ai.Client
	topK  int
}

func New(store *db.Store, client *ai.Client, topK int) *Engine {
	if topK <= 0 {
		topK = 6
	}
	return &Engine{store: store, ai: client, topK: topK}
}

// Ingest parses, chunks, embeds and stores one markdown document.
func (e *Engine) Ingest(ctx context.Context, path, content string) (int, error) {
	chunks := SplitMarkdown(content)
	if len(chunks) == 0 {
		return 0, nil
	}

	// Embed everything *before* touching the database. Writing the document row
	// first would leave a chunk-less row behind whenever the provider fails — a
	// document the corpus lists and retrieval can never return.
	const batch = 64 // stay under per-request provider limits
	vectors := make([][]float32, 0, len(chunks))
	for i := 0; i < len(chunks); i += batch {
		end := min(i+batch, len(chunks))
		inputs := make([]string, 0, end-i)
		for _, c := range chunks[i:end] {
			inputs = append(inputs, c.Heading+"\n"+c.Content)
		}
		vecs, err := e.ai.Embed(ctx, inputs)
		if err != nil {
			return 0, err
		}
		if len(vecs) != len(inputs) {
			return 0, fmt.Errorf("embed: %d vectors for %d chunks", len(vecs), len(inputs))
		}
		vectors = append(vectors, vecs...)
	}

	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	docID, err := e.store.UpsertDocument(path, title)
	if err != nil {
		return 0, err
	}
	for i, c := range chunks {
		if err := e.store.InsertChunk(docID, c.Heading, c.Content, i, vectors[i]); err != nil {
			return 0, err
		}
	}
	return len(chunks), nil
}

// Citation points a claim back to its source chunk.
type Citation struct {
	N       int    `json:"n"`
	DocPath string `json:"doc"`
	Heading string `json:"heading"`
}

const systemPrompt = `You are a precise technical knowledge assistant for an internal engineering team.

RULES:
- Answer ONLY from the CONTEXT below. Never use outside knowledge.
- If the context does not contain the answer, say exactly: "Không tìm thấy thông tin này trong tài liệu."
- Be concise and scientific. Prefer short paragraphs, code, and bullet points.
- Cite every claim with [n] referring to the numbered sources.
- Answer in the language of the question.`

// Answer retrieves context and streams a grounded reply. Citations are returned
// after streaming completes so the UI can render source links.
func (e *Engine) Answer(ctx context.Context, question string, onToken func(string)) ([]Citation, error) {
	qv, err := e.ai.Embed(ctx, []string{question})
	if err != nil {
		return nil, err
	}
	hits, err := e.store.Search(qv[0], question, e.topK)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		onToken("Không tìm thấy thông tin này trong tài liệu.")
		return nil, nil
	}

	var cb strings.Builder
	cites := make([]Citation, len(hits))
	for i, h := range hits {
		n := i + 1
		fmt.Fprintf(&cb, "[%d] source: %s | section: %s\n%s\n\n", n, h.DocPath, h.Heading, h.Content)
		cites[i] = Citation{N: n, DocPath: h.DocPath, Heading: h.Heading}
	}

	msgs := []ai.Msg{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "CONTEXT:\n" + cb.String() + "\nQUESTION: " + question},
	}
	if err := e.ai.ChatStream(ctx, msgs, onToken); err != nil {
		return cites, err
	}
	return cites, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Corpus reports what has been indexed. It's a thin pass-through, but it keeps
// the HTTP layer talking to one thing (the engine) instead of reaching for the
// store directly.
func (e *Engine) Corpus(limit int) (db.Corpus, error) { return e.store.Corpus(limit) }
