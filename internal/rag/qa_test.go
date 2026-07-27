package rag_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/aitest"
	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
)

/* ══ The answer cache ═══════════════════════════════════════════════════════════
   Every test here is about money: a cache that doesn't save a provider call is
   decoration, and a cache that serves an answer whose sources have moved is worse
   than no cache at all. Both are asserted against the fake provider's own call log
   rather than a flag we set ourselves. */

func TestARepeatQuestionCostsNothing(t *testing.T) {
	e, prov := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	first, reply, err := ask(t, e, "How does hybrid search rank results?")
	if err != nil {
		t.Fatalf("first ask: %v", err)
	}
	if reply.Cached {
		t.Fatal("the first ask cannot be a cache hit")
	}
	chats, embeds, cites := len(prov.Chats()), len(prov.Embedded()), reply.Citations

	second, reply, err := ask(t, e, "  How Does HYBRID search rank results?  ")
	if err != nil {
		t.Fatalf("second ask: %v", err)
	}
	if !reply.Cached {
		t.Error("the same question, differing only in case and spacing, was not a hit")
	}
	if second != first {
		t.Errorf("cached answer differs:\n first  = %q\n second = %q", first, second)
	}
	// The [n] markers in the cached text point at these rows; losing them would
	// leave the answer citing nothing.
	if len(reply.Citations) != len(cites) || len(cites) == 0 {
		t.Errorf("citations: %d cached vs %d fresh", len(reply.Citations), len(cites))
	}
	for i, c := range reply.Citations {
		if c != cites[i] {
			t.Errorf("citation %d changed: %+v vs %+v", i+1, c, cites[i])
		}
	}
	if got := len(prov.Chats()); got != chats {
		t.Errorf("a cache hit still spent %d completion(s)", got-chats)
	}
	if got := len(prov.Embedded()); got != embeds {
		t.Errorf("a cache hit still spent %d embedding call(s)", got-embeds)
	}
}

func TestRegenerateIgnoresTheCache(t *testing.T) {
	e, prov := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ask(t, e, "hybrid search?"); err != nil {
		t.Fatal(err)
	}
	before := len(prov.Chats())

	reply, err := e.Answer(ctx, rag.Ask{Question: "hybrid search?", Fresh: true})
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if reply.Cached {
		t.Error("Regenerate returned the cached answer — the user just said it was wrong")
	}
	if len(prov.Chats()) != before+1 {
		t.Error("Regenerate did not reach the model")
	}
}

func TestIndexingInvalidatesTheCache(t *testing.T) {
	e, prov := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ask(t, e, "hybrid search?"); err != nil {
		t.Fatal(err)
	}
	before := len(prov.Chats())

	// New documents can change the answer, so the old one must not be reused.
	if _, err := e.Ingest(ctx, "docs/deploy.md", deployDoc); err != nil {
		t.Fatal(err)
	}
	_, reply, err := ask(t, e, "hybrid search?")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Cached {
		t.Error("an answer survived a re-index; its citations may no longer be true")
	}
	if len(prov.Chats()) != before+1 {
		t.Error("the question was not re-answered after the corpus changed")
	}
	// And the history reflects the corpus in front of it, not the one behind it.
	h, err := e.History(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 1 {
		t.Errorf("history has %d entries; the pre-index answer should be gone", len(h))
	}
}

func TestAMissIsNeverCached(t *testing.T) {
	e, _ := engine(t, nil) // nothing ingested: retrieval finds nothing

	if _, _, err := ask(t, e, "anything?"); err != nil {
		t.Fatal(err)
	}
	h, err := e.History(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 0 {
		t.Errorf(`"not in the documents" was cached: %+v — the BA loop exists to fix that, and a cache would hide the fix`, h)
	}
}

func TestHistoryCountsHitsSoTheSavingIsVisible(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, _, err := ask(t, e, "hybrid search?"); err != nil {
			t.Fatal(err)
		}
	}

	h, err := e.History(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 1 {
		t.Fatalf("history = %d entries for one repeated question", len(h))
	}
	if h[0].Hits != 2 { // three asks: one paid, two free
		t.Errorf("hits = %d, want 2 — that count is the only visible proof of the saving", h[0].Hits)
	}
	if h[0].Answer == "" {
		t.Error("a history entry with no answer cannot be replayed for free")
	}
}

/* ══ The BA ⇄ DEV loop ══════════════════════════════════════════════════════════
   The claim under test is end-to-end: a question the documents cannot answer comes
   back, after one BA confirm, as a cited answer for the next person who asks. */

func TestConfirmedAnswerBecomesAFileAndThenACitation(t *testing.T) {
	corpus := t.TempDir()
	e, prov := engineIn(t, &aitest.Provider{Reply: "Invoices are void after 30 days [1]."}, corpus)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	const q = "How long is an invoice valid?"
	ticket, err := e.OpenTicket(q, rag.NoAnswer)
	if err != nil {
		t.Fatalf("open ticket: %v", err)
	}
	if ticket.Status != db.StatusOpen {
		t.Errorf("a new ticket is %q, want open", ticket.Status)
	}

	const answer = "An invoice is valid for 30 days from the issue date, then it is void."
	ticket, err = e.Confirm(ctx, ticket.ID, answer)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if ticket.Status != db.StatusConfirmed {
		t.Errorf("status after confirm = %q", ticket.Status)
	}

	// 1. It is a file on disk, so `ingest` rebuilds it and git can review it. This
	//    is what keeps the database derived rather than a second source of truth.
	onDisk := filepath.Join(corpus, ticket.DocPath)
	body, err := os.ReadFile(onDisk)
	if err != nil {
		t.Fatalf("the confirmed answer is not on disk: %v", err)
	}
	if !strings.Contains(string(body), answer) || !strings.Contains(string(body), q) {
		t.Errorf("the file carries neither the question nor the answer:\n%s", body)
	}

	// 2. It is retrievable, and cited by the path the file actually has.
	text, reply, err := ask(t, e, "How long is an invoice valid?")
	if err != nil {
		t.Fatalf("ask after confirm: %v", err)
	}
	if text == rag.NoAnswer {
		t.Fatal("the confirmed answer was not retrieved")
	}
	cited := false
	for _, c := range reply.Citations {
		if c.DocPath == ticket.DocPath {
			cited = true
		}
	}
	if !cited {
		t.Errorf("the confirmed document is not among the citations: %+v", reply.Citations)
	}
	if !strings.Contains(strings.Join(prov.Chats(), "\n"), answer) {
		t.Error("the BA's answer never reached the model as context")
	}

	// 3. Its chunks are approved — the retrieval boost that only a human confirm
	//    can earn.
	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Approved == 0 {
		t.Error("confirmed chunks are still draft, so the SoT boost does nothing")
	}
}

func TestTheSameGapFiledTwiceIsOneTicket(t *testing.T) {
	e, _ := engine(t, nil)

	first, err := e.OpenTicket("Where is the refund policy?", rag.NoAnswer)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.OpenTicket("  where IS the Refund policy?  ", rag.NoAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("two tickets (#%d, #%d) for one question — the BA reads the same gap twice",
			first.ID, second.ID)
	}

	q, err := e.Queue(0)
	if err != nil {
		t.Fatal(err)
	}
	if q.Open != 1 || len(q.Tickets) != 1 {
		t.Errorf("queue = %d open / %d listed", q.Open, len(q.Tickets))
	}
}

func TestASettledTicketDoesNotBlockTheNextOne(t *testing.T) {
	e, _ := engine(t, nil)

	const q = "Which team owns billing?"
	first, err := e.OpenTicket(q, rag.NoAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Reject(first.ID, "belongs in the org chart, not the docs"); err != nil {
		t.Fatal(err)
	}
	// Asked again later: the dismissed ticket must not silently swallow it.
	again, err := e.OpenTicket(q, rag.NoAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID == first.ID {
		t.Error("the question was folded into a ticket that is already closed")
	}
	if again.Status != db.StatusOpen {
		t.Errorf("the new ticket is %q", again.Status)
	}
}

func TestADraftSurvivesAndStaysOutOfRetrieval(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()

	ticket, err := e.OpenTicket("What is the SLA?", rag.NoAnswer)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err = e.Draft(ticket.ID, "Four hours, business days.")
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if ticket.Status != db.StatusAnswered || ticket.Answer == "" {
		t.Errorf("draft left the ticket as %q with answer %q", ticket.Status, ticket.Answer)
	}
	if ticket.DocPath != "" {
		t.Error("a draft published a document; only a confirm may do that")
	}
	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Docs != 0 {
		t.Error("a draft reached the corpus")
	}

	// The BA comes back and publishes it.
	if ticket, err = e.Confirm(ctx, ticket.ID, ticket.Answer); err != nil {
		t.Fatalf("confirm after draft: %v", err)
	}
	if ticket.Status != db.StatusConfirmed {
		t.Errorf("status = %q", ticket.Status)
	}
}

func TestConfirmingTwiceIsRefused(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()

	ticket, err := e.OpenTicket("Retention period?", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(ctx, ticket.ID, "Seven years."); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(ctx, ticket.ID, "Actually five."); err == nil {
		t.Error("a second confirm was accepted; the first answer is already cited elsewhere")
	}
}

func TestConfirmRefusesAnEmptyAnswer(t *testing.T) {
	e, _ := engine(t, nil)
	ticket, err := e.OpenTicket("Anything?", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(context.Background(), ticket.ID, "   \n "); err == nil {
		t.Error("an empty answer was indexed as knowledge")
	}
}

func TestConfirmWithoutACorpusDirectoryFailsBeforeIndexing(t *testing.T) {
	e, prov := engineIn(t, nil, "") // CORPUS_DIR unset
	ticket, err := e.OpenTicket("Where do confirmed answers go?", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.Confirm(context.Background(), ticket.ID, "Into docs/qa/.")
	if err == nil {
		t.Fatal("want a refusal: an answer indexed with no file behind it is unreproducible")
	}
	if !strings.Contains(err.Error(), "CORPUS_DIR") {
		t.Errorf("the error should name the fix; got: %v", err)
	}
	if len(prov.Embedded()) != 0 {
		t.Error("it spent an embeddings call before discovering it had nowhere to write")
	}
}

func TestConfirmedAnswerIsReproducibleByIngest(t *testing.T) {
	corpus := t.TempDir()
	e, _ := engineIn(t, nil, corpus)
	ctx := context.Background()

	ticket, err := e.OpenTicket("What does the QA loop write?", "")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err = e.Confirm(ctx, ticket.ID, "A markdown file under docs/qa/.")
	if err != nil {
		t.Fatal(err)
	}

	// The whole point of writing a file: a second engine, given only the directory,
	// arrives at the same corpus. That is what makes knowledge.db disposable.
	fresh, _ := engineIn(t, nil, corpus)
	body, err := os.ReadFile(filepath.Join(corpus, ticket.DocPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Ingest(ctx, ticket.DocPath, string(body)); err != nil {
		t.Fatalf("re-ingesting the confirmed file: %v", err)
	}
	c, err := fresh.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Docs != 1 || c.Chunks == 0 {
		t.Errorf("rebuilt corpus = %d docs / %d chunks", c.Docs, c.Chunks)
	}
}

// The cache must not survive a model change. Reported from a real deployment: after
// switching CHAT_MODEL the app kept answering from the old model, which reads as the
// setting doing nothing.
func TestChangingTheChatModelInvalidatesTheCache(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "models.db"), dim)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	prov, base := aitest.New(&aitest.Provider{Dim: dim})
	t.Cleanup(prov.Close)
	engineFor := func(model string) *rag.Engine {
		return rag.New(store, ai.New(ai.Config{
			ChatBaseURL: base, APIKey: "test-key", EmbedModel: "embed-model", ChatModel: model,
		}), rag.Options{TopK: 3, CorpusDir: dir})
	}
	old, updated := engineFor("gpt-4o-mini"), engineFor("gpt-4.1")

	ctx := context.Background()
	if _, err := old.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	const q = "hybrid search?"
	if _, _, err := ask(t, old, q); err != nil {
		t.Fatal(err)
	}
	if _, reply, _ := ask(t, old, q); !reply.Cached {
		t.Fatal("same model, same question: expected a cache hit to compare against")
	}
	calls := len(prov.Chats())

	_, reply, err := ask(t, updated, q)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Cached {
		t.Error("the new model served the old model's answer from cache")
	}
	if len(prov.Chats()) != calls+1 {
		t.Error("the new model never reached the provider")
	}
}
