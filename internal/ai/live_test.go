//go:build live

// Live provider check. Never runs in `make check` — it needs a real key and a
// real network, so it sits behind the `live` build tag:
//
//	make live                      # reads .env
//	AI_BASE_URL=… AI_API_KEY=… make live
//
// It answers the question you actually have when pointing this app at a new
// provider: does it speak both endpoints, what embedding width does it return,
// and does chat really stream? Failures name the fix rather than the symptom.
package ai_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/config"
)

func liveClient(t *testing.T) (*ai.Client, config.Config) {
	t.Helper()
	cfg := config.Load() // reads .env, so the key never has to be on a command line
	if cfg.APIKey == "" {
		t.Skip("no AI_API_KEY — set it in .env (see .env.example) to run the live check")
	}
	t.Logf("chat:       %s  (%s)", cfg.BaseURL, cfg.ChatModel)
	embed := cfg.EmbedURL
	if embed == "" {
		embed = cfg.BaseURL + "  (same as chat)"
	}
	t.Logf("embeddings: %s  (%s)", embed, cfg.EmbedModel)
	return ai.New(ai.Config{ChatBaseURL: cfg.BaseURL, EmbedBaseURL: cfg.EmbedURL, APIKey: cfg.APIKey, EmbedModel: cfg.EmbedModel, ChatModel: cfg.ChatModel}), cfg
}

func TestLiveEmbeddings(t *testing.T) {
	c, cfg := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inputs := []string{"hybrid retrieval", "deployment"}
	vecs, err := c.Embed(ctx, inputs)
	if err != nil {
		// The single most common failure when adding a coding-agent gateway.
		if strings.Contains(err.Error(), "no embeddings endpoint") {
			t.Fatalf("%s has no /embeddings.\n"+
				"Ingest cannot work without it. Set EMBED_BASE_URL to a provider that does —\n"+
				"e.g. EMBED_BASE_URL=http://localhost:11434/v1 with EMBED_MODEL=nomic-embed-text\n"+
				"and EMBED_DIM=768 (Ollama, local, free) — and leave AI_BASE_URL for chat.\n\n%v",
				cfg.BaseURL, err)
		}
		t.Fatalf("embeddings failed: %v", err)
	}
	if len(vecs) != len(inputs) {
		t.Fatalf("got %d vectors for %d inputs", len(vecs), len(inputs))
	}

	got := len(vecs[0])
	t.Logf("embedding width: %d", got)
	if got != cfg.EmbedDim {
		t.Fatalf("EMBED_DIM is %d but %s returns %d.\n"+
			"Set EMBED_DIM=%d and re-ingest (delete %s first) — a mismatch fails every insert.",
			cfg.EmbedDim, cfg.EmbedModel, got, got, cfg.DBPath)
	}
}

func TestLiveChatStreams(t *testing.T) {
	c, _ := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var chunks int
	var reply strings.Builder
	first := time.Time{}
	start := time.Now()

	err := c.ChatStream(ctx, []ai.Msg{
		{Role: "system", Content: "Answer in exactly three words."},
		{Role: "user", Content: "Name three colours."},
	}, func(tok string) {
		if first.IsZero() {
			first = time.Now()
		}
		chunks++
		reply.WriteString(tok)
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if reply.Len() == 0 {
		t.Fatal("the provider streamed no content — check CHAT_MODEL")
	}
	t.Logf("first token in %v, %d chunks, %d chars: %q",
		first.Sub(start).Round(time.Millisecond), chunks, reply.Len(), reply.String())

	// One chunk means the provider buffered the whole reply; the UI would sit on a
	// spinner and then paste everything at once. Worth knowing, not worth failing.
	if chunks == 1 {
		t.Log("note: the reply arrived as a single chunk — this provider does not stream incrementally")
	}
}
