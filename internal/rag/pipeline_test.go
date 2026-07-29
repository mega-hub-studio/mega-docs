package rag_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/aitest"
	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
)

const dim = 16

// engine wires the real store and the real ai.Client against a fake provider —
// so this exercises every layer the product does, minus the model itself.
//
// There used to be an engineIn() beside this, taking a corpus directory, for the tests that
// looked at what a confirm wrote to disk. Nothing writes to disk any more, so the parameter
// and its variant are gone rather than accepted and ignored.
func engine(t *testing.T, p *aitest.Provider) (*rag.Engine, *aitest.Provider) {
	t.Helper()
	if p == nil {
		p = &aitest.Provider{}
	}
	p.Dim = dim
	prov, base := aitest.New(p)
	t.Cleanup(prov.Close)

	store, err := db.Open(filepath.Join(t.TempDir(), "pipeline.db"), dim)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	client := ai.New(ai.Config{
		ChatBaseURL: base, APIKey: "test-key",
		EmbedModel: "embed-model", ChatModel: "chat-model",
	})
	return rag.New(store, client, rag.Options{TopK: 3}), prov
}

// ask is the common case: one question, the streamed text collected.
func ask(t *testing.T, e *rag.Engine, question string) (string, rag.Reply, error) {
	t.Helper()
	var sb strings.Builder
	reply, err := e.Answer(context.Background(), rag.Ask{
		Question: question,
		OnToken:  func(tok string) { sb.WriteString(tok) },
	})
	return sb.String(), reply, err
}

// askIn is ask() with a retrieval scope.
func askIn(t *testing.T, e *rag.Engine, question, scope string) (string, rag.Reply, error) {
	t.Helper()
	var sb strings.Builder
	reply, err := e.Answer(context.Background(), rag.Ask{
		Question: question, Scope: scope,
		OnToken: func(tok string) { sb.WriteString(tok) },
	})
	return sb.String(), reply, err
}

const retrievalDoc = `# Retrieval

## Hybrid search
Retrieval fuses vector similarity and BM25 keyword matching with Reciprocal Rank
Fusion. Keyword matching catches error codes and config keys that pure semantic
search misses.

## Approval
Chunks carry a status of draft or approved, and approved chunks are boosted.
`

const deployDoc = `# Deployment

## Binary
Everything ships in one Go binary; the frontend is embedded.
`

func TestIngestThenAnswerCitesTheIngestedDocument(t *testing.T) {
	e, prov := engine(t, &aitest.Provider{
		Reply: "Retrieval fuses vector similarity and BM25 with RRF [1].",
	})
	ctx := context.Background()

	n, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n == 0 {
		t.Fatal("ingest produced no chunks")
	}
	if _, err := e.Ingest(ctx, "docs/deploy.md", deployDoc); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}

	var tokens []string
	reply, err := e.Answer(ctx, rag.Ask{
		Question: "How does hybrid search rank results?",
		OnToken:  func(tok string) { tokens = append(tokens, tok) },
	})
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	cites := reply.Citations

	if got := strings.Join(tokens, ""); !strings.Contains(got, "RRF") {
		t.Errorf("streamed answer = %q", got)
	}
	if len(tokens) < 3 {
		t.Errorf("answer arrived in %d pieces; it should stream", len(tokens))
	}
	if reply.Cached {
		t.Error("a first-time question was reported as cached")
	}
	if len(cites) == 0 {
		t.Fatal("a grounded answer with no citations is the bug this whole app exists to avoid")
	}

	// Citations must be numbered from 1 and point at real ingested files.
	for i, c := range cites {
		if c.N != i+1 {
			t.Errorf("citation %d numbered %d — the UI links [n] by position", i, c.N)
		}
		if c.DocPath == "" {
			t.Errorf("citation %d has no document path", c.N)
		}
	}
	// The question is about hybrid search, so that document must be cited.
	found := false
	for _, c := range cites {
		if c.DocPath == "docs/retrieval.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("retrieval.md was not retrieved for a question about hybrid search: %+v", cites)
	}

	// The model must have been handed the retrieved context, not just the question.
	chats := prov.Chats()
	if len(chats) != 1 {
		t.Fatalf("expected one chat call, got %d", len(chats))
	}
	if !strings.Contains(chats[0], "CONTEXT:") || !strings.Contains(chats[0], "Reciprocal Rank") {
		t.Error("the prompt did not carry the retrieved chunks")
	}
	if !strings.Contains(chats[0], "How does hybrid search rank results?") {
		t.Error("the prompt did not carry the question")
	}
}

func TestAnswerOnAnEmptyIndexSaysSoAndCitesNothing(t *testing.T) {
	e, prov := engine(t, nil)

	got, reply, err := ask(t, e, "anything?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(reply.Citations) != 0 {
		t.Errorf("citations on an empty index: %+v", reply.Citations)
	}
	if got == "" {
		t.Error("the user must be told something, not left with a blank answer")
	}
	// No retrieval, no reason to spend a model call.
	if len(prov.Chats()) != 0 {
		t.Error("an empty index still called the chat model")
	}
}

// Ingest batches embeddings; a document larger than one batch must still end up
// with every chunk indexed and correctly paired with its own vector.
func TestIngestBatchesLargeDocumentsWithoutMisalignment(t *testing.T) {
	e, prov := engine(t, nil)
	ctx := context.Background()

	var sb strings.Builder
	sb.WriteString("# Big\n\n")
	for i := range 70 {
		// Distinct heading + body so a mispaired vector is detectable, and each body
		// over minChars so the chunker leaves 70 sections as 70 chunks — the point of
		// this test is the batching, not the merging.
		sb.WriteString("## Section ")
		sb.WriteString(strings.Repeat("x", i%5+1))
		sb.WriteString("\n")
		sb.WriteString("Body about topic")
		sb.WriteString(strings.Repeat("y", i%7+1))
		sb.WriteString(" number ")
		sb.WriteString(strings.Repeat("z", i%3+1))
		sb.WriteString(". ")
		sb.WriteString(strings.Repeat("padding so this section stands on its own. ", 15))
		sb.WriteString("\n\n")
	}

	n, err := e.Ingest(ctx, "docs/big.md", sb.String())
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n < 2 {
		t.Fatalf("expected many chunks, got %d", n)
	}

	batches := prov.Embedded()
	if len(batches) < 2 {
		t.Fatalf("expected more than one embedding batch for %d chunks, got %d", n, len(batches))
	}
	total := 0
	for _, b := range batches {
		if len(b) > 64 {
			t.Errorf("batch of %d exceeds the provider-friendly cap", len(b))
		}
		total += len(b)
	}
	if total != n {
		t.Errorf("embedded %d texts for %d chunks", total, n)
	}
}

func TestIngestFailsLoudlyWhenTheProviderHasNoEmbeddings(t *testing.T) {
	e, _ := engine(t, &aitest.Provider{NoEmbeddings: true})

	_, err := e.Ingest(context.Background(), "docs/x.md", retrievalDoc)
	if err == nil {
		t.Fatal("want an error rather than an empty index")
	}
	if !strings.Contains(err.Error(), "EMBED_BASE_URL") {
		t.Errorf("the error should point at the fix; got: %v", err)
	}
}

func TestAnswerReportsAChatFailureAfterRetrievalSucceeded(t *testing.T) {
	e, _ := engine(t, &aitest.Provider{ChatStatus: 503})
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	_, reply, err := ask(t, e, "hybrid search?")
	if err == nil {
		t.Fatal("want the provider failure surfaced")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error lost the status: %v", err)
	}
	// Retrieval worked, so the citations are still worth returning — the UI can
	// show which sources it *would* have used.
	if len(reply.Citations) == 0 {
		t.Error("citations were dropped even though retrieval succeeded")
	}
}

func TestIngestOfAnEmptyDocumentIsANoOp(t *testing.T) {
	e, prov := engine(t, nil)

	n, err := e.Ingest(context.Background(), "docs/empty.md", "   \n\n  ")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n != 0 {
		t.Errorf("chunked %d pieces out of whitespace", n)
	}
	if len(prov.Embedded()) != 0 {
		t.Error("an empty document still spent an embeddings call")
	}
}

// Re-ingesting the same path must not duplicate the document row.
func TestReIngestKeepsOneDocumentRow(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()

	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Docs != 1 {
		t.Errorf("corpus reports %d documents after re-ingesting one path", c.Docs)
	}
}

// A provider failure must leave no trace: a document row with zero chunks would be
// listed by the corpus and returned by nothing, which reads as "ready but broken".
func TestFailedIngestLeavesNoPhantomDocument(t *testing.T) {
	e, _ := engine(t, &aitest.Provider{NoEmbeddings: true})

	if _, err := e.Ingest(context.Background(), "docs/retrieval.md", retrievalDoc); err == nil {
		t.Fatal("expected the ingest to fail")
	}

	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Docs != 0 || c.Chunks != 0 {
		t.Errorf("failed ingest left %d documents / %d chunks behind", c.Docs, c.Chunks)
	}
}

// Retrieval finding chunks and the model finding no answer in them are different
// events, and only the second one reaches the reader. Printing the sources anyway is
// a contradiction on screen — "this is not in the documents", followed by six places
// to go and look for it.
func TestAMissCitesNothingEvenWhenRetrievalFoundChunks(t *testing.T) {
	e, prov := engine(t, &aitest.Provider{Reply: rag.NoAnswer})
	ctx := context.Background()

	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	got, reply, err := ask(t, e, "what is the refund policy?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	// The model was asked — this is not the empty-index shortcut.
	if len(prov.Chats()) != 1 {
		t.Fatalf("chat calls = %d, want 1 (retrieval must have produced context)", len(prov.Chats()))
	}
	if got != rag.NoAnswer {
		t.Fatalf("answer = %q, want the no-answer sentence", got)
	}
	if len(reply.Citations) != 0 {
		t.Errorf("a miss cited %d source(s): %+v", len(reply.Citations), reply.Citations)
	}
}

// TOP_K is a retrieval budget, not a reading list. An answer that used one section
// must not hand the reader five more to check — and the numbers it did use have to
// keep working as links, so nothing may be renumbered.
func TestOnlyTheSourcesTheAnswerCitedAreReturned(t *testing.T) {
	e, _ := engine(t, &aitest.Provider{Reply: "Approved chunks are boosted [2]."})
	ctx := context.Background()
	for _, d := range []struct{ path, body string }{
		{"docs/retrieval.md", retrievalDoc},
		{"docs/deploy.md", deployDoc},
	} {
		if _, err := e.Ingest(ctx, d.path, d.body); err != nil {
			t.Fatalf("ingest %s: %v", d.path, err)
		}
	}

	_, reply, err := ask(t, e, "are approved chunks boosted?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(reply.Citations) != 1 {
		t.Fatalf("answer cited [2] only, but %d source(s) came back: %+v", len(reply.Citations), reply.Citations)
	}
	if reply.Citations[0].N != 2 {
		t.Errorf("citation renumbered to %d — the answer text says [2] and the link would miss", reply.Citations[0].N)
	}
}

// An answer with no markers has the source list as its only provenance; guessing
// which entry mattered would be worse than showing them all.
func TestAnAnswerThatCitesNothingKeepsEverySource(t *testing.T) {
	e, _ := engine(t, &aitest.Provider{Reply: "Approved chunks are boosted."})
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	_, reply, err := ask(t, e, "are approved chunks boosted?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(reply.Citations) == 0 {
		t.Error("an uncited answer was left with no provenance at all")
	}
}

// engineWithModels is `engine` for the two things a model list changes: which window a thread
// is trimmed to, and which key an answer is cached under. Same fake provider, same store.
func engineWithModels(t *testing.T, p *aitest.Provider, models []rag.Model) (*rag.Engine, *aitest.Provider) {
	t.Helper()
	if p == nil {
		p = &aitest.Provider{}
	}
	p.Dim = dim
	prov, base := aitest.New(p)
	t.Cleanup(prov.Close)

	store, err := db.Open(filepath.Join(t.TempDir(), "models.db"), dim)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	client := ai.New(ai.Config{
		ChatBaseURL: base, APIKey: "test-key",
		EmbedModel: "embed-model", ChatModel: "chat-model",
	})
	return rag.New(store, client, rag.Options{TopK: 3, Models: models}), prov
}
