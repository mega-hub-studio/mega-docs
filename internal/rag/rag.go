package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/db"
)

type Engine struct {
	store     *db.Store
	ai        *ai.Client
	topK      int
	corpusDir string
}

// Options is everything the engine needs besides its store and provider. A struct,
// so adding a knob doesn't rewrite every call site.
type Options struct {
	TopK      int    // chunks per answer; <=0 means 6
	CorpusDir string // where documents live on disk; "" disables writes (Confirm)
}

func New(store *db.Store, client *ai.Client, opt Options) *Engine {
	if opt.TopK <= 0 {
		opt.TopK = 6
	}
	return &Engine{store: store, ai: client, topK: opt.TopK, corpusDir: opt.CorpusDir}
}

// Ingest parses, chunks, embeds and stores one markdown document. The title is the
// file name — the only thing a path reliably tells you about a document.
func (e *Engine) Ingest(ctx context.Context, path, content string) (int, error) {
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return e.ingest(ctx, path, title, content)
}

func (e *Engine) ingest(ctx context.Context, path, title, content string) (int, error) {
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

// NoAnswer is what the engine says when the documents don't cover the question. It
// is one string because three things depend on it agreeing: the prompt, the
// no-retrieval shortcut, and the rule that a miss is never cached.
const NoAnswer = "Không tìm thấy thông tin này trong tài liệu."

const systemPrompt = `You are a precise technical knowledge assistant for an internal engineering team.

RULES:
- Answer ONLY from the CONTEXT below. Never use outside knowledge.
- If the context does not contain the answer, say exactly: "` + NoAnswer + `"
- Be concise and scientific. Prefer short paragraphs, code, and bullet points.
- Cite every claim with [n] referring to the numbered sources.
- Answer in the language of the question.`

// Ask is one question and how to answer it. A struct rather than three parameters
// because the next flag added here shouldn't change every call site.
type Ask struct {
	Question string
	Fresh    bool         // ignore any cached answer — what Regenerate means
	OnToken  func(string) // may be nil
}

// Reply is what the engine produced. Cached is surfaced to the UI on purpose: a
// free answer that looks identical to a paid one teaches nobody anything.
type Reply struct {
	Citations []Citation `json:"citations"`
	Cached    bool       `json:"cached"`
}

// Answer retrieves context and streams a grounded reply. Citations come back after
// streaming so the UI can render source links.
//
// A repeat question is served from the cache: no embedding call, no completion, no
// cost. The key is the question, the corpus signature and the chat model — so
// re-indexing *or* changing the model invalidates it, rather than serving an answer
// whose sources have moved or whose model has been replaced.
func (e *Engine) Answer(ctx context.Context, a Ask) (Reply, error) {
	question := strings.TrimSpace(a.Question)
	onToken := a.OnToken
	if onToken == nil {
		onToken = func(string) {}
	}

	// A signature we can't read means "don't cache", never "fail the question".
	sig, sigErr := e.sig()
	if sigErr == nil && !a.Fresh {
		if c, ok, err := e.store.Cached(sig, question); err == nil && ok {
			onToken(c.Answer)
			var cites []Citation
			_ = json.Unmarshal(c.Citations, &cites)
			return Reply{Citations: cites, Cached: true}, nil
		}
	}

	qv, err := e.ai.Embed(ctx, []string{question})
	if err != nil {
		return Reply{}, err
	}
	hits, err := e.store.Search(qv[0], question, e.topK)
	if err != nil {
		return Reply{}, err
	}
	if len(hits) == 0 {
		onToken(NoAnswer)
		return Reply{}, nil
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
	// Accumulate what the user is already seeing: the cache stores the answer, and
	// re-serialising it from the model would cost the tokens twice.
	var full strings.Builder
	err = e.ai.ChatStream(ctx, msgs, func(tok string) {
		full.WriteString(tok)
		onToken(tok)
	})
	if err != nil {
		return Reply{Citations: cites}, err
	}
	// Only cache a grounded, complete answer. A cut-off stream or an ungrounded
	// reply is exactly what someone will retry, and a cache that remembers it turns
	// one bad answer into a permanent one.
	if sigErr == nil && full.Len() > 0 && !strings.Contains(full.String(), NoAnswer) {
		raw, _ := json.Marshal(cites)
		_ = e.store.Cache(sig, db.Cached{Question: question, Answer: full.String(), Citations: raw})
	}
	return Reply{Citations: cites}, nil
}

// Corpus reports what has been indexed. It's a thin pass-through, but it keeps
// the HTTP layer talking to one thing (the engine) instead of reaching for the
// store directly.
func (e *Engine) Corpus(limit int) (db.Corpus, error) { return e.store.Corpus(limit) }
