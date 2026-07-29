package rag_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
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
	for range 3 {
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

func TestConfirmedAnswerBecomesADocumentAndThenACitation(t *testing.T) {
	e, prov := engine(t, &aitest.Provider{Reply: "Invoices are void after 30 days [1]."})
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

	// 1. It is a document in the knowledge base, body and all — the inversion. This used to
	//    assert a file in CORPUS_DIR, which was what kept the database derived; the database
	//    is the source of truth now, so the row *is* the document and there is nothing else
	//    to reconcile it with.
	doc, ok, err := e.Document(ticket.DocPath)
	if err != nil || !ok {
		t.Fatalf("the confirmed answer is not in the knowledge base (ok=%v): %v", ok, err)
	}
	if !strings.Contains(doc.Body, answer) || !strings.Contains(doc.Body, q) {
		t.Errorf("the stored document carries neither the question nor the answer:\n%s", doc.Body)
	}

	// 2. It is retrievable, and cited by the path the document is stored under.
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

func TestConfirmingTwiceCorrectsTheAnswerInPlace(t *testing.T) {
	// This used to assert the opposite, on the theory that the first answer was already
	// cited elsewhere. What that bought was a published mistake nobody could fix: `confirmed`
	// refused every transition, so the only remedy was to go and find the document in the
	// library — and removing it there left the ticket still claiming to be in the knowledge
	// base. Correcting in place is the cheaper honesty: the path comes from the id, so the
	// fix lands exactly where the citation already points.
	e, _ := engine(t, nil)
	ctx := context.Background()

	ticket, err := e.OpenTicket("Retention period?", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(ctx, ticket.ID, "Seven years."); err != nil {
		t.Fatal(err)
	}
	corrected, err := e.Confirm(ctx, ticket.ID, "Actually five.")
	if err != nil {
		t.Fatalf("a correction was refused: %v", err)
	}
	if corrected.Answer != "Actually five." || corrected.Status != db.StatusConfirmed {
		t.Errorf("ticket after the correction = %+v", corrected)
	}

	doc, ok, err := e.Document(corrected.DocPath)
	if err != nil || !ok {
		t.Fatalf("the corrected answer is not in the knowledge base (ok=%v): %v", ok, err)
	}
	if !strings.Contains(doc.Body, "Actually five.") {
		t.Errorf("the document still holds the old answer:\n%s", doc.Body)
	}
	if strings.Contains(doc.Body, "Seven years.") {
		t.Errorf("both answers are in one document — a correction must replace, not append:\n%s", doc.Body)
	}
}

func TestTakingAConfirmedAnswerBackOutStopsItAnswering(t *testing.T) {
	// The three edges out of `confirmed`, and the obligation they share: the document leaves
	// retrieval before the ticket moves, so the queue and the corpus can never disagree about
	// whether an answer is live. Retract keeps the words so the BA edits rather than retypes.
	e, _ := engine(t, nil)
	ctx := context.Background()

	ticket, err := e.OpenTicket("Retention period?", "")
	if err != nil {
		t.Fatal(err)
	}
	if ticket, err = e.Confirm(ctx, ticket.ID, "Seven years."); err != nil {
		t.Fatal(err)
	}
	path := ticket.DocPath

	retracted, err := e.Retract(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if retracted.Status != db.StatusAnswered {
		t.Errorf("status after retract = %q, want the draft back", retracted.Status)
	}
	if retracted.DocPath != "" {
		t.Errorf("the ticket still names %q — a retracted answer documents nothing", retracted.DocPath)
	}
	if retracted.Answer != "Seven years." {
		t.Errorf("retract lost the answer (%q); the BA edits it, they do not retype it", retracted.Answer)
	}
	if inCorpus(t, e, path) {
		t.Errorf("%s is still retrievable after a retract", path)
	}
	// The text survives: removal is a deleted_at column, and that is the only way back.
	if _, ok, _ := e.Document(path); !ok {
		t.Errorf("%s lost its text — a retract must not destroy what a BA wrote", path)
	}

	// Retracting what was never published is the client's stale view, not a state change.
	if _, err := e.Retract(ctx, retracted.ID); err == nil {
		t.Error("retracting a draft was accepted")
	}
}

func TestDeletingATicketDropsTheQuestionAndKeepsTheWords(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()

	ticket, err := e.OpenTicket("Retention period?", "")
	if err != nil {
		t.Fatal(err)
	}
	if ticket, err = e.Confirm(ctx, ticket.ID, "Seven years."); err != nil {
		t.Fatal(err)
	}
	path := ticket.DocPath

	if err := e.Delete(ctx, ticket.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	q, err := e.Queue(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Tickets) != 0 || q.Confirmed != 0 {
		t.Errorf("the queue still holds %d tickets (confirmed=%d) after a delete", len(q.Tickets), q.Confirmed)
	}
	if inCorpus(t, e, path) {
		t.Errorf("%s goes on answering questions its ticket no longer exists to explain", path)
	}
	if _, ok, _ := e.Document(path); !ok {
		t.Errorf("%s lost its text; deleting a ticket costs the question, never the answer", path)
	}
	if err := e.Delete(ctx, ticket.ID); !errors.Is(err, db.ErrNoTicket) {
		t.Errorf("deleting it twice = %v, want ErrNoTicket", err)
	}
}

func TestDismissingAConfirmedTicketUnpublishesIt(t *testing.T) {
	// A dismissal that left the answer answering would be a dismissal in the queue only —
	// the reader would still be cited a document the BA had just judged wrong.
	e, _ := engine(t, nil)
	ctx := context.Background()

	ticket, err := e.OpenTicket("Retention period?", "")
	if err != nil {
		t.Fatal(err)
	}
	if ticket, err = e.Confirm(ctx, ticket.ID, "Seven years."); err != nil {
		t.Fatal(err)
	}
	path := ticket.DocPath

	dismissed, err := e.Reject(ticket.ID, "Legal owns this, not us.")
	if err != nil {
		t.Fatalf("dismissing a confirmed ticket: %v", err)
	}
	if dismissed.Status != db.StatusRejected {
		t.Errorf("status = %q, want rejected", dismissed.Status)
	}
	if inCorpus(t, e, path) {
		t.Errorf("%s is dismissed and still retrievable", path)
	}
}

// inCorpus reports whether a path is still one of the documents retrieval can reach. The
// corpus listing is the honest question to ask: Document() reads a removed row on purpose.
func inCorpus(t *testing.T, e *rag.Engine, path string) bool {
	t.Helper()
	c, err := e.Corpus(0)
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(c.Documents, func(d db.Document) bool { return d.Path == path })
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

// Two tests stood here and are deleted rather than adapted, because the properties they
// enforced no longer exist:
//
//   TestConfirmWithoutACorpusDirectoryFailsBeforeIndexing — there is no corpus directory to
//   be without. A confirm needed one because the file was the source of truth and an
//   unreproducible index was the failure; the row is the document now.
//
//   TestConfirmedAnswerIsReproducibleByIngest — "a second engine, given only the directory,
//   arrives at the same corpus" was what made knowledge.db disposable. It is not disposable
//   any more, which is the whole inversion, and a test asserting otherwise would be a lie in
//   the gate rather than a check.
//
// What replaces them: TestConfirmedAnswerBecomesADocumentAndThenACitation above (the answer
// is a document, retrievable and cited) and TestARemovedDocumentStopsAnsweringAndItsTextSurvives
// in remove_test.go (the only safety net there is now).

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
		}), rag.Options{TopK: 3})
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

/* ══ Conversation memory ════════════════════════════════════════════════════════
   A thread buys two things that must both hold, because either one alone is a bug:
   retrieval has to run on what a follow-up *means*, and the cache has to stay out of
   it in both directions. "còn bước 2 thì sao?" is five words that mean something
   different in every conversation, so a row keyed on them is a wrong answer waiting
   for the next person who types them. */

// The acceptance test from changelog/2026-07-28-memory-and-external-search.md: the
// second answer has to cite a section the second question's own words could never have
// retrieved.
func TestAFollowUpIsRewrittenForRetrievalAndNeverCached(t *testing.T) {
	// The fake answers every chat call with this — the rewrite call included — so it
	// doubles as the rewritten question, and a retrieval that ran on the rewrite shows
	// up in the embedding log as this exact text.
	const rewritten = "Which rule covers an unpaid deposit after 24 hours?"
	e, prov := engine(t, &aitest.Provider{Reply: rewritten})
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "docs/retrieval.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}

	const first = "How does hybrid search rank results?"
	answer, _, err := ask(t, e, first)
	if err != nil {
		t.Fatal(err)
	}

	// A follow-up with no content word in it: it embeds to nothing useful and gives BM25
	// nothing to match, so retrieval on its own text would find the wrong sections or none.
	const followUp = "còn bước 2 thì sao?"
	embeds, chats := len(prov.Embedded()), len(prov.Chats())
	reply, err := e.Answer(ctx, rag.Ask{
		Question: followUp,
		History:  []rag.Turn{{Q: first, A: answer}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := len(prov.Chats()) - chats; got != 2 {
		t.Fatalf("a follow-up bought %d completions, want 2: one to rewrite it, one to answer it", got)
	}
	if got := len(prov.Embedded()) - embeds; got != 1 {
		t.Fatalf("a follow-up made %d embedding calls, want 1", got)
	}
	if asked := prov.Embedded()[len(prov.Embedded())-1]; len(asked) != 1 || asked[0] != rewritten {
		t.Errorf("retrieval embedded %q; the follow-up's own words retrieve nothing, so the rewrite is what had to be embedded", asked)
	}
	// The rewrite is asked for separately, and it is asked about the follow-up itself —
	// no CONTEXT, because there is nothing to retrieve with yet.
	if got := prov.Chats()[len(prov.Chats())-2]; got != followUp {
		t.Errorf("the rewrite call asked about %q, not the follow-up", got)
	}
	// And the answering call is given the conversation, not just the sections: without
	// this the answer has the context retrieval found and none of what was said above it.
	sent := prov.Messages()[len(prov.Messages())-1]
	for _, want := range []string{"user: " + first, "assistant: " + answer} {
		if !slices.Contains(sent, want) {
			t.Errorf("the answering call never saw %q", want)
		}
	}

	if reply.Cached {
		t.Error("a follow-up was served from the cache — that row belongs to whichever conversation stored it")
	}
	// Nothing was stored under those five words either. The same text in a fresh
	// conversation is a different question, and it must be answered rather than replayed.
	if _, reply, err := ask(t, e, followUp); err != nil || reply.Cached {
		t.Errorf("the follow-up was cached under its own text: cached=%v err=%v", reply.Cached, err)
	}
}

/* ══ Scoped retrieval ═══════════════════════════════════════════════════════════
   A scope is a promise: answer from *this* part of the corpus. Two things can break
   it — retrieval reaching outside the scope, and the cache answering a scoped
   question from an unscoped row. Both are asserted here, because a violation of
   either produces a confident answer citing documents the reader excluded. */

func TestAScopeKeepsTheAnswerInsideIt(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "booking/rules.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ingest(ctx, "support/deploy.md", deployDoc); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ name, scope, want string }{
		{"unscoped reaches both documents", "", ""},
		{"a folder scope", "support", "support/deploy.md"},
		{"the same folder, written loosely", "/support/", "support/deploy.md"},
		{"one document", "booking/rules.md", "booking/rules.md"},
	} {
		_, reply, err := askIn(t, e, "what ships in the binary?", c.scope)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(reply.Citations) == 0 {
			t.Fatalf("%s: nothing cited", c.name)
		}
		if c.want == "" {
			continue
		}
		for _, cite := range reply.Citations {
			if cite.DocPath != c.want {
				t.Errorf("%s: cited %s, which is outside the scope %q", c.name, cite.DocPath, c.scope)
			}
		}
	}
}

func TestTheSameQuestionInAnotherScopeIsAnotherAnswer(t *testing.T) {
	e, prov := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "booking/rules.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ingest(ctx, "support/deploy.md", deployDoc); err != nil {
		t.Fatal(err)
	}

	const q = "what ships in the binary?"
	if _, reply, err := askIn(t, e, q, "support"); err != nil || reply.Cached {
		t.Fatalf("first scoped ask: cached=%v err=%v", reply.Cached, err)
	}
	spent := len(prov.Chats())

	// Same words, different scope: this must be answered afresh, from the other
	// folder — not handed the row the first ask stored.
	_, reply, err := askIn(t, e, q, "booking")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Cached {
		t.Error("a different scope was served from another scope's cached answer")
	}
	if len(prov.Chats()) == spent {
		t.Error("no completion was bought, so the answer came from the wrong scope's row")
	}
	for _, c := range reply.Citations {
		if c.DocPath != "booking/rules.md" {
			t.Errorf("cited %s under scope booking", c.DocPath)
		}
	}

	// And the scope's own repeat is still free.
	if _, reply, err := askIn(t, e, q, "booking"); err != nil || !reply.Cached {
		t.Errorf("repeating a scoped question was not free: cached=%v err=%v", reply.Cached, err)
	}
	// The unscoped question is a third identity, and is not answered by either row.
	if _, reply, err := askIn(t, e, q, ""); err != nil || reply.Cached {
		t.Errorf("the unscoped question was served from a scoped row: cached=%v err=%v", reply.Cached, err)
	}
}

// The History panel is what makes a free answer visible, and a scoped row is only
// free when replayed in the same scope — so the scope has to survive the round trip.
func TestHistoryRemembersTheScopeItWasAnsweredIn(t *testing.T) {
	e, _ := engine(t, nil)
	ctx := context.Background()
	if _, err := e.Ingest(ctx, "booking/rules.md", retrievalDoc); err != nil {
		t.Fatal(err)
	}
	if _, _, err := askIn(t, e, "how does hybrid search rank?", "booking"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := askIn(t, e, "what is approval?", ""); err != nil {
		t.Fatal(err)
	}

	rows, err := e.History(10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Question] = r.Scope
	}
	if got["how does hybrid search rank?"] != "booking" {
		t.Errorf("scoped row came back with scope %q; want booking", got["how does hybrid search rank?"])
	}
	if got["what is approval?"] != "" {
		t.Errorf("unscoped row came back with scope %q; want empty", got["what is approval?"])
	}
	// The question text itself must stay clean — the key carries the scope, the
	// display text does not.
	for q := range got {
		if strings.ContainsRune(q, '\x1f') {
			t.Errorf("history question leaked the key separator: %q", q)
		}
	}
}
