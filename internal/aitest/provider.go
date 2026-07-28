// Package aitest serves a fake OpenAI-compatible provider over httptest, so the
// real ai.Client — and everything built on it — can be exercised end to end with
// no API key and no network.
//
// It is deliberately a *server*, not a stub of the client: the request encoding,
// the `index` ordering of embeddings, and SSE frame parsing are exactly the parts
// most likely to break against a new provider, and a hand-written fake client
// would test none of them.
package aitest

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Provider is a configurable fake. Zero value is usable via New.
type Provider struct {
	// Dim is the embedding width. Must match the store's dimension.
	Dim int
	// Reply is streamed back, split on spaces, one SSE frame per token.
	Reply string

	// Fault injection — each mirrors a real provider behaviour we have to survive.
	NoEmbeddings   bool // 404 on /embeddings, like a chat-only gateway
	ShuffleIndexes bool // return embeddings out of order (the API allows it)
	DropOneVector  bool // return fewer vectors than inputs
	MidStreamError string
	ChatStatus     int // non-zero to fail /chat/completions with this status

	mu       sync.Mutex
	embedded [][]string // every batch of inputs it was asked to embed
	chats    []string   // every chat request's last user message
	tokens   []string   // the bearer token of every request, in order
	server   *httptest.Server
}

// New starts a provider and returns it with the server's base URL.
func New(p *Provider) (*Provider, string) {
	if p == nil {
		p = &Provider{}
	}
	if p.Dim == 0 {
		p.Dim = 4
	}
	if p.Reply == "" {
		p.Reply = "Retrieval fuses vector and keyword search [1]."
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", p.embeddings)
	mux.HandleFunc("/chat/completions", p.chat)
	p.server = httptest.NewServer(mux)
	return p, p.server.URL
}

// Close shuts the test server down. Safe to call twice.
func (p *Provider) Close() { p.server.Close() }

// Embedded reports the input batches the provider was asked to embed.
func (p *Provider) Embedded() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.embedded
}

// Tokens reports the bearer token of every request this provider received. Chat and
// embeddings can be two different vendors, so a test needs to know that each host
// was handed its own key and not the other's.
func (p *Provider) Tokens() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokens
}

// Chats reports the user message of each chat request, in order.
func (p *Provider) Chats() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.chats
}

// record notes the bearer token and reports whether one was present at all.
func (p *Provider) record(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	p.mu.Lock()
	p.tokens = append(p.tokens, strings.TrimPrefix(auth, "Bearer "))
	p.mu.Unlock()
	return true
}

func (p *Provider) embeddings(w http.ResponseWriter, r *http.Request) {
	if p.NoEmbeddings {
		http.Error(w, `{"error":{"message":"unknown endpoint"}}`, http.StatusNotFound)
		return
	}
	if !p.record(r) {
		http.Error(w, `{"error":{"message":"missing key"}}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"bad json"}}`, http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	p.embedded = append(p.embedded, body.Input)
	p.mu.Unlock()

	type item struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}
	items := make([]item, 0, len(body.Input))
	for i, in := range body.Input {
		items = append(items, item{Index: i, Embedding: Vector(in, p.Dim)})
	}
	if p.ShuffleIndexes && len(items) > 1 {
		// reverse: array order now disagrees with `index`, which is legal
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	if p.DropOneVector && len(items) > 0 {
		items = items[:len(items)-1]
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": items})
}

func (p *Provider) chat(w http.ResponseWriter, r *http.Request) {
	// Recorded, not required: the missing-key path is covered on /embeddings, and
	// demanding one here would change what existing chat tests exercise.
	p.record(r)
	if p.ChatStatus != 0 {
		http.Error(w, `{"error":{"message":"upstream is down"}}`, p.ChatStatus)
		return
	}

	var body struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"bad json"}}`, http.StatusBadRequest)
		return
	}
	last := ""
	for _, m := range body.Messages {
		if m.Role == "user" {
			last = m.Content
		}
	}
	p.mu.Lock()
	p.chats = append(p.chats, last)
	p.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	frame := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// A real stream is noisy: comment lines and blank separators show up between
	// frames, and the client has to skip them.
	_, _ = fmt.Fprint(w, ": ping\n\n")
	for i, tok := range strings.Fields(p.Reply) {
		if i > 0 {
			tok = " " + tok
		}
		frame(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": tok}}}})
	}
	if p.MidStreamError != "" {
		frame(map[string]any{"error": map[string]string{"message": p.MidStreamError}})
		return
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

// Vector is a deterministic pseudo-embedding: same text in, same vector out, and
// texts sharing words land near each other, which is enough for a retrieval test
// to be meaningful rather than arbitrary.
func Vector(text string, dim int) []float32 {
	v := make([]float32, dim)
	for word := range strings.FieldsSeq(strings.ToLower(text)) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(word)) // hash writes never fail
		v[int(h.Sum32())%dim]++
	}
	// keep it unit-ish so cosine/L2 distances behave
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		v[0] = 1
		return v
	}
	mag := sqrt32(sum)
	for i := range v {
		v[i] /= mag
	}
	return v
}

func sqrt32(f float32) float32 {
	if f <= 0 {
		return 1
	}
	// Newton, plenty for a test fixture
	x := f
	for range 12 {
		x = 0.5 * (x + f/x)
	}
	return x
}
