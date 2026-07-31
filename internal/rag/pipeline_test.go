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

// wideCorpus is five documents of three retrievable sections each — enough that the
// per-document cap binds and a budget has something to widen into. The filler is what keeps
// each section above SplitMarkdown's minChars, so it stands alone instead of merging with
// its neighbour and turning fifteen sections into five.
func wideCorpus(t *testing.T, e *rag.Engine) {
	t.Helper()
	filler := strings.Repeat("Retrieval fuses vector distance with keyword rank. ", 14)
	for _, doc := range []string{"booking", "billing", "support", "auth", "reporting"} {
		var b strings.Builder
		b.WriteString("# " + doc + "\n\n")
		for _, section := range []string{"overview", "rules", "edge cases"} {
			b.WriteString("## " + section + "\n\n" + doc + " " + section + ": " + filler + "\n\n")
		}
		if _, err := e.Ingest(context.Background(), "docs/"+doc+".md", b.String()); err != nil {
			t.Fatalf("ingest %s: %v", doc, err)
		}
	}
}

// TestRetrievalWidensToTheModelsWindow is the check that an answer is as wide as the model
// can read rather than as wide as one number nobody re-tuned.
//
// TOP_K was six against windows that hold forty: the instance paid for a reader and used a
// skim. So the count follows CONTEXT_SHARE of the picked model's window when the operator has
// said what that window is — and, when they have not, it is still exactly TOP_K, because a
// retrieval width that quietly depended on an optional display knob would make an
// unconfigured instance answer worse with nothing saying so.
func TestRetrievalWidensToTheModelsWindow(t *testing.T) {
	for _, c := range []struct {
		name   string
		window int
		want   func(got int) bool
		wantIf string
	}{
		{"no window declared: TOP_K, exactly as before", 0,
			func(got int) bool { return got == 3 }, "3"},
		{"a real window: as many sections as it holds", 100_000,
			func(got int) bool { return got > 3 }, "more than TOP_K's 3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, _ := engineWithModels(t, &aitest.Provider{Reply: "grounded [1]"},
				[]rag.Model{{Name: "chat-model", Window: c.window}})
			wideCorpus(t, e)

			_, reply, err := ask(t, e, "what are the rules for booking?")
			if err != nil {
				t.Fatalf("answer: %v", err)
			}
			if !c.want(reply.Retrieval.Kept) {
				t.Errorf("read %d sections; want %s", reply.Retrieval.Kept, c.wantIf)
			}
			if reply.Retrieval.Offered < reply.Retrieval.Kept {
				t.Errorf("read %d of %d offered — a budget cannot keep more than it weighed",
					reply.Retrieval.Kept, reply.Retrieval.Offered)
			}
		})
	}
}

// engineWithSearch is `engine` with the public-search supplement switched on, pointed at the
// same fake provider — one server, one call log, so a test can assert what was *not* called.
//
// The model list is a parameter because it decides how many sections retrieval asks for, and
// that is the number the trigger compares against: a declared window asks for the whole
// candidate pool, no window asks for TOP_K.
func engineWithSearch(t *testing.T, p *aitest.Provider, models []rag.Model) (*rag.Engine, *aitest.Provider) {
	t.Helper()
	if p == nil {
		p = &aitest.Provider{}
	}
	p.Dim = dim
	prov, base := aitest.New(p)
	t.Cleanup(prov.Close)

	store, err := db.Open(filepath.Join(t.TempDir(), "search.db"), dim)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	client := ai.New(ai.Config{
		ChatBaseURL: base, APIKey: "test-key",
		EmbedModel: "embed-model", ChatModel: "chat-model",
	})
	return rag.New(store, client, rag.Options{
		TopK: 3, Models: models,
		SearchBaseURL: base, SearchAPIKey: "search-key",
	}), prov
}

// TestAMissReachesABANotTheWeb is the guarantee the whole supplement is built around: a gap
// belongs to a BA, because that route ends in the documents *being able to cover it*, and a
// web result answers the person in front of you and nobody after them.
//
// Two halves, because only one of them can be a call log. Retrieval returning nothing is
// decided before any provider is called, so that one is asserted as "the search endpoint was
// never reached". A model that reads real sections and then declares a miss is decided
// *after* the prompt was built, so no pre-generation gate can see it coming — what holds
// there is that the reply is still the bare sentence with no sources of any kind under it,
// which is what keeps Ask BA the only thing on screen to do next.
func TestAMissReachesABANotTheWeb(t *testing.T) {
	t.Run("retrieval found nothing: no external call at all", func(t *testing.T) {
		e, prov := engineWithSearch(t, &aitest.Provider{Reply: "unreachable"}, []rag.Model{{Name: "chat-model", Window: 100_000}})
		if _, err := e.Ingest(context.Background(), "docs/retrieval.md", retrievalDoc); err != nil {
			t.Fatal(err)
		}
		// A scope no document lives under: both retrievers are filtered before they rank, so
		// this is the zero-hit path rather than a weak-hit one.
		got, _, err := askIn(t, e, "what is our refund window?", "billing/enterprise")
		if err != nil {
			t.Fatalf("answer: %v", err)
		}
		if got != rag.NoAnswer {
			t.Fatalf("answer = %q; want the no-answer sentence", got)
		}
		if n := len(prov.Searches()); n != 0 {
			t.Errorf("a question with no sections at all bought %d public searches: %v",
				n, prov.Searches())
		}
	})

	t.Run("the model declared the miss: nothing is cited under it", func(t *testing.T) {
		e, _ := engineWithSearch(t, &aitest.Provider{Reply: rag.NoAnswer}, []rag.Model{{Name: "chat-model", Window: 100_000}})
		if _, err := e.Ingest(context.Background(), "docs/retrieval.md", retrievalDoc); err != nil {
			t.Fatal(err)
		}
		got, reply, err := ask(t, e, "what is our refund window for enterprise contracts?")
		if err != nil {
			t.Fatalf("answer: %v", err)
		}
		if got != rag.NoAnswer {
			t.Fatalf("answer = %q; want the no-answer sentence", got)
		}
		// Not one document, and not one link either. "This is not in the documents" over a
		// list of web results is an invitation to read the web result as the answer.
		if len(reply.Citations) != 0 {
			t.Errorf("a miss carried %d citations: %+v", len(reply.Citations), reply.Citations)
		}
	})
}

// TestACorpusThatDidNotRunOutIsNotSupplemented is the trigger's other side, and it is the one
// that decides the bill: retrieval coming back full means the corpus was not the limit, so
// there is nothing to supplement and no reason to send the question anywhere.
//
// Deliberately on an instance with no declared window, because that is where the numbers are
// small enough to be obvious: retrieval is asked for TOP_K, the corpus has more than TOP_K
// matching sections, so what comes back is exactly TOP_K and the comparison is an equality
// rather than a threshold. The first version of this trigger measured retrieved characters
// against a fraction of the model's context window, and against a 128k window that called
// almost every answer thin — every question would have gone to a third party.
func TestACorpusThatDidNotRunOutIsNotSupplemented(t *testing.T) {
	e, prov := engineWithSearch(t,
		&aitest.Provider{Reply: "Entirely from the documents [1]."},
		[]rag.Model{{Name: "chat-model", Window: 0}})
	wideCorpus(t, e)

	_, reply, err := ask(t, e, "what are the rules for booking?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if reply.Retrieval.Offered != 3 {
		t.Fatalf("retrieval offered %d sections; this test needs it to come back full at TOP_K's 3",
			reply.Retrieval.Offered)
	}
	if n := len(prov.Searches()); n != 0 {
		t.Errorf("a corpus that filled its own request bought %d public searches: %v", n, prov.Searches())
	}
}

// TestWebResultsAreCitedInTheirOwnNumbering is the provenance half. A sentence from a search
// API and a sentence from a specification a person approved must not render identically, so
// the web gets its own markers, its own list, and its own kind on the wire.
//
// The narrowing rule is the opposite of a document's, and that is the assertion at the end: an
// answer that cited no web result shows none, because printing links under an answer that did
// not use them claims a supplement that never happened.
func TestWebResultsAreCitedInTheirOwnNumbering(t *testing.T) {
	e, prov := engineWithSearch(t, &aitest.Provider{
		Reply: "The documents cover the handshake [1]; the standard behind it is OAuth 2.0 [w1].",
		SearchResults: []aitest.SearchHit{{
			Title: "OAuth 2.0", URL: "https://example.test/oauth",
			Content: "OAuth 2.0 is an authorisation framework for delegated access.",
		}},
	}, []rag.Model{{Name: "chat-model", Window: 100_000}})
	if _, err := e.Ingest(context.Background(), "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	_, reply, err := ask(t, e, "how does the handshake work?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if len(prov.Searches()) == 0 {
		t.Fatal("a one-document corpus cannot fill a 40-candidate pool, so it ran out and no search ran")
	}
	var doc, web []rag.Citation
	for _, c := range reply.Citations {
		if c.Kind == "web" {
			web = append(web, c)
			continue
		}
		doc = append(doc, c)
	}
	if len(doc) == 0 {
		t.Error("the answer cited [1] and no document came back — a supplement replaced the grounding")
	}
	if len(web) != 1 {
		t.Fatalf("web citations = %d; want the one the answer cited as [w1]", len(web))
	}
	if web[0].URL == "" || web[0].DocPath != "" {
		t.Errorf("web citation = %+v; want a URL and no document path", web[0])
	}
	// The same corpus, the same search, an answer that used neither web result.
	e2, _ := engineWithSearch(t, &aitest.Provider{Reply: "Entirely from the documents [1]."}, []rag.Model{{Name: "chat-model", Window: 100_000}})
	if _, err := e2.Ingest(context.Background(), "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	_, plain, err := ask(t, e2, "how does the handshake work?")
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	for _, c := range plain.Citations {
		if c.Kind == "web" {
			t.Errorf("an answer citing no [wN] still listed %+v — that claims a supplement it did not use", c)
		}
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
