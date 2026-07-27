package ai_test

import (
	"context"
	"strings"
	"testing"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/aitest"
)

func newClient(t *testing.T, p *aitest.Provider) *ai.Client {
	t.Helper()
	prov, base := aitest.New(p)
	t.Cleanup(prov.Close)
	return ai.New(ai.Config{ChatBaseURL: base, APIKey: "test-key", EmbedModel: "embed-model", ChatModel: "chat-model"})
}

func TestEmbedReturnsOneVectorPerInputInOrder(t *testing.T) {
	prov, base := aitest.New(&aitest.Provider{Dim: 8})
	defer prov.Close()
	c := ai.New(ai.Config{ChatBaseURL: base, APIKey: "k", EmbedModel: "embed-model", ChatModel: "chat-model"})

	inputs := []string{"alpha", "beta", "gamma"}
	vecs, err := c.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != len(inputs) {
		t.Fatalf("got %d vectors for %d inputs", len(vecs), len(inputs))
	}
	for i, v := range vecs {
		if len(v) != 8 {
			t.Errorf("vector %d has width %d, want 8", i, len(v))
		}
		// Each vector must be the one for *its* input, not just any vector.
		want := aitest.Vector(inputs[i], 8)
		for j := range want {
			if v[j] != want[j] {
				t.Errorf("vector %d doesn't match input %q", i, inputs[i])
				break
			}
		}
	}
	if got := prov.Embedded(); len(got) != 1 || len(got[0]) != 3 {
		t.Errorf("provider saw %v, want one batch of 3", got)
	}
}

// The API orders results by `index`, not by array position. If the client trusted
// array order, embeddings would silently attach to the wrong chunks — an index
// that looks healthy and retrieves nonsense.
func TestEmbedRespectsIndexNotArrayOrder(t *testing.T) {
	c := newClient(t, &aitest.Provider{Dim: 8, ShuffleIndexes: true})

	inputs := []string{"first", "second", "third"}
	vecs, err := c.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	for i, in := range inputs {
		want := aitest.Vector(in, 8)
		for j := range want {
			if vecs[i][j] != want[j] {
				t.Fatalf("vector %d is not %q's — the client used array order, not index", i, in)
			}
		}
	}
}

func TestEmbedRejectsAShortResponse(t *testing.T) {
	// Silently accepting fewer vectors than inputs would leave chunks unindexed.
	c := newClient(t, &aitest.Provider{Dim: 4, DropOneVector: true})

	_, err := c.Embed(context.Background(), []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("want an error when the provider returns fewer vectors than inputs")
	}
	if !strings.Contains(err.Error(), "asked for 3") {
		t.Errorf("error should say what was missing, got: %v", err)
	}
}

func TestEmbedOnEmptyInputMakesNoCall(t *testing.T) {
	prov, base := aitest.New(nil)
	defer prov.Close()
	c := ai.New(ai.Config{ChatBaseURL: base, APIKey: "k", EmbedModel: "e", ChatModel: "c"})

	vecs, err := c.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Fatalf("got %v, %v; want nil, nil", vecs, err)
	}
	if len(prov.Embedded()) != 0 {
		t.Error("an empty batch still hit the provider")
	}
}

// A chat-only gateway is the common case that breaks a RAG install, so the error
// has to name the cause and the fix rather than just echoing "404".
func TestEmbedExplainsAProviderWithoutEmbeddings(t *testing.T) {
	c := newClient(t, &aitest.Provider{NoEmbeddings: true})

	_, err := c.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"no embeddings endpoint", "EMBED_BASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q; got: %v", want, err)
		}
	}
}

func TestChatStreamDeliversTokensInOrderAndSkipsNoise(t *testing.T) {
	prov, base := aitest.New(&aitest.Provider{Reply: "Hybrid search wins [1]."})
	defer prov.Close()
	c := ai.New(ai.Config{ChatBaseURL: base, APIKey: "k", EmbedModel: "e", ChatModel: "chat-model"})

	var got []string
	err := c.ChatStream(context.Background(),
		[]ai.Msg{{Role: "system", Content: "ctx"}, {Role: "user", Content: "why?"}},
		func(tok string) { got = append(got, tok) })
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(got, ""); joined != "Hybrid search wins [1]." {
		t.Errorf("assembled %q", joined)
	}
	if len(got) < 4 {
		t.Errorf("got %d tokens; the stream should arrive incrementally, not at once", len(got))
	}
	if chats := prov.Chats(); len(chats) != 1 || chats[0] != "why?" {
		t.Errorf("provider saw %v, want the user message", chats)
	}
}

// Some providers report a failure mid-stream as a data frame with HTTP 200.
// Ignoring it would surface as a truncated answer with no error at all.
func TestChatStreamSurfacesAMidStreamError(t *testing.T) {
	c := newClient(t, &aitest.Provider{Reply: "partial answer", MidStreamError: "context length exceeded"})

	var got string
	err := c.ChatStream(context.Background(), []ai.Msg{{Role: "user", Content: "q"}},
		func(tok string) { got += tok })
	if err == nil {
		t.Fatal("want an error from the mid-stream frame")
	}
	if !strings.Contains(err.Error(), "context length exceeded") {
		t.Errorf("error lost the provider's message: %v", err)
	}
	if got == "" {
		t.Error("tokens before the error should still have been delivered")
	}
}

func TestChatStreamReportsHTTPFailureWithTheBaseURL(t *testing.T) {
	c := newClient(t, &aitest.Provider{ChatStatus: 502})

	err := c.ChatStream(context.Background(), []ai.Msg{{Role: "user", Content: "q"}}, func(string) {})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("error should name the status and the endpoint: %v", err)
	}
}

func TestEmbedAndChatCanUseDifferentProviders(t *testing.T) {
	// The whole point of the split: chat somewhere without embeddings, embeddings
	// somewhere that has them.
	chatOnly, chatBase := aitest.New(&aitest.Provider{NoEmbeddings: true, Reply: "ok"})
	defer chatOnly.Close()
	embedder, embedBase := aitest.New(&aitest.Provider{Dim: 4})
	defer embedder.Close()

	c := ai.New(ai.Config{ChatBaseURL: chatBase, EmbedBaseURL: embedBase, APIKey: "k", EmbedModel: "e", ChatModel: "c"})

	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("embeddings should have gone to the embed provider: %v", err)
	}
	if err := c.ChatStream(context.Background(), []ai.Msg{{Role: "user", Content: "q"}}, func(string) {}); err != nil {
		t.Fatalf("chat should have gone to the chat provider: %v", err)
	}
	if len(embedder.Embedded()) != 1 || len(chatOnly.Chats()) != 1 {
		t.Error("a request went to the wrong provider")
	}
}

func TestTrailingSlashesInBaseURLsDoNotDoubleUp(t *testing.T) {
	prov, base := aitest.New(&aitest.Provider{Dim: 4})
	defer prov.Close()
	c := ai.New(ai.Config{ChatBaseURL: base + "/", EmbedBaseURL: base + "/", APIKey: "k", EmbedModel: "e", ChatModel: "c"})

	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Errorf("a trailing slash broke the URL: %v", err)
	}
}

func TestMissingAPIKeySendsNoAuthorizationHeader(t *testing.T) {
	// A local Ollama needs no key; sending "Bearer " would be worse than nothing.
	// The fake rejects a malformed Bearer header, so a 401 here proves it was absent.
	c := newClient(t, &aitest.Provider{Dim: 4})
	keyless := ai.New(ai.Config{
		ChatBaseURL: strings.TrimSuffix(c.EmbedBaseURL, "/"),
		EmbedModel:  "e", ChatModel: "c",
	})

	_, err := keyless.Embed(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("want the fake's 401 (no auth header sent), got: %v", err)
	}
}

// Splitting the base URL is only half of it: if chat and embeddings are different
// vendors, one key is not valid at both. Each host must get its own.
func TestEmbedAndChatCanUseDifferentKeys(t *testing.T) {
	chatOnly, chatBase := aitest.New(&aitest.Provider{NoEmbeddings: true, Reply: "ok"})
	defer chatOnly.Close()
	embedder, embedBase := aitest.New(&aitest.Provider{Dim: 4})
	defer embedder.Close()

	c := ai.New(ai.Config{
		ChatBaseURL: chatBase, EmbedBaseURL: embedBase,
		APIKey: "chat-key", EmbedAPIKey: "embed-key",
		EmbedModel: "e", ChatModel: "c",
	})
	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err := c.ChatStream(context.Background(),
		[]ai.Msg{{Role: "user", Content: "q"}}, func(string) {}); err != nil {
		t.Fatal(err)
	}

	if got := embedder.Tokens(); len(got) != 1 || got[0] != "embed-key" {
		t.Errorf("embed provider saw %v, want [embed-key] — the chat key leaked", got)
	}
	if got := chatOnly.Tokens(); len(got) != 1 || got[0] != "chat-key" {
		t.Errorf("chat provider saw %v, want [chat-key]", got)
	}
}

// The common case is one provider for both, so an unset embed key must fall back
// rather than sending nothing and 401ing.
func TestEmbedKeyDefaultsToTheMainKey(t *testing.T) {
	prov, base := aitest.New(&aitest.Provider{Dim: 4})
	defer prov.Close()
	c := ai.New(ai.Config{ChatBaseURL: base, APIKey: "only-key", EmbedModel: "e", ChatModel: "c"})

	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if got := prov.Tokens(); len(got) != 1 || got[0] != "only-key" {
		t.Errorf("embed request carried %v, want [only-key]", got)
	}
}
