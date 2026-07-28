package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"knowledge-engine/internal/db"
)

// fakeKnow records what the HTTP layer asked of the QA loop. It answers rather
// than validates: the transitions themselves are the store's tests.
type fakeKnow struct {
	queue    db.Queue
	ticket   db.Ticket
	history  []db.Cached
	err      error
	confirms []string // the answers a confirm was called with, in order
	opened   [][2]string
}

func (f *fakeKnow) Queue(int) (db.Queue, error) { return f.queue, f.err }

func (f *fakeKnow) OpenTicket(q, miss string) (db.Ticket, error) {
	f.opened = append(f.opened, [2]string{q, miss})
	return f.ticket, f.err
}

func (f *fakeKnow) Draft(_ int64, a string) (db.Ticket, error) {
	f.ticket.Answer, f.ticket.Status = a, db.StatusAnswered
	return f.ticket, f.err
}

func (f *fakeKnow) Confirm(_ context.Context, _ int64, a string) (db.Ticket, error) {
	f.confirms = append(f.confirms, a)
	f.ticket.Answer, f.ticket.Status = a, db.StatusConfirmed
	return f.ticket, f.err
}

func (f *fakeKnow) Reject(_ int64, note string) (db.Ticket, error) {
	f.ticket.Note, f.ticket.Status = note, db.StatusRejected
	return f.ticket, f.err
}

func (f *fakeKnow) History(int) ([]db.Cached, error) { return f.history, f.err }

func qaServer(k Knowledge, pass BAPass) http.Handler {
	return New(Deps{
		Answers: &fakeAnswers{},
		Know:    k,
		Index:   []byte("<html>index</html>"),
		Assets:  fstest.MapFS{"assets/index-A1b2C3d4.js": {Data: []byte("export const x = 1\n")}},
		BAPass:  pass,
	})
}

const baPass = "s3cret-ba"

func withPass(p string) map[string]string {
	return map[string]string{"X-BA-Pass": p}
}

/* ══ The gate ══════════════════════════════════════════════════════════════════
   Reads open, writes gated. This app has no accounts, so this header is the whole
   difference between "the tailnet can read the documents" and "the tailnet can
   rewrite them". Each case below is a way that could quietly stop being true. */

func TestFilingAGapNeedsNoPassword(t *testing.T) {
	k := &fakeKnow{ticket: db.Ticket{ID: 7, Status: db.StatusOpen}}
	h := qaServer(k, baPass)

	w := do(t, h, "POST", "/api/tickets", `{"question":"How long is an invoice valid?","miss":"not in the documents"}`, nil)
	if w.Code != 200 {
		t.Fatalf("POST /api/tickets = %d %s", w.Code, w.Body.String())
	}
	if len(k.opened) != 1 || k.opened[0][0] == "" || k.opened[0][1] == "" {
		t.Errorf("the gap reached the engine as %+v — the miss is the BA's evidence", k.opened)
	}
	var got db.Ticket
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.ID != 7 {
		t.Errorf("the DEV must get their ticket back: %s (%v)", w.Body.String(), err)
	}
}

func TestReadingTheQueueAndHistoryNeedsNoPassword(t *testing.T) {
	k := &fakeKnow{
		queue:   db.Queue{Open: 2, Tickets: []db.Ticket{{ID: 1, Status: db.StatusOpen}}},
		history: []db.Cached{{Question: "q", Answer: "a", Hits: 3}},
	}
	h := qaServer(k, baPass)

	for _, path := range []string{"/api/tickets", "/api/history"} {
		if w := do(t, h, "GET", path, "", nil); w.Code != 200 {
			t.Errorf("GET %s = %d; reads must stay open", path, w.Code)
		}
	}
}

func TestConfirmAndRejectRequireThePassword(t *testing.T) {
	for _, action := range []string{"confirm", "reject", "draft"} {
		k := &fakeKnow{ticket: db.Ticket{ID: 3}}
		h := qaServer(k, baPass)
		path := "/api/tickets/3/" + action

		if w := do(t, h, "POST", path, `{"answer":"x"}`, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s with no password = %d, want 401", action, w.Code)
		}
		if w := do(t, h, "POST", path, `{"answer":"x"}`, withPass("wrong")); w.Code != http.StatusUnauthorized {
			t.Errorf("%s with the wrong password = %d, want 401", action, w.Code)
		}
		if w := do(t, h, "POST", path, `{"answer":"x"}`, withPass(baPass)); w.Code != 200 {
			t.Errorf("%s with the password = %d, want 200 (%s)", action, w.Code, w.Body.String())
		}
	}
}

func TestWithoutBAPassThereIsNoWriteSurfaceAtAll(t *testing.T) {
	// Forgetting to set a secret must never be how an instance ends up without one.
	k := &fakeKnow{ticket: db.Ticket{ID: 3}}
	h := qaServer(k, "")

	for _, hdr := range []map[string]string{nil, withPass(""), withPass("anything")} {
		w := do(t, h, "POST", "/api/tickets/3/confirm", `{"answer":"x"}`, hdr)
		if w.Code != http.StatusForbidden {
			t.Errorf("confirm on a read-only instance = %d, want 403", w.Code)
		}
		if !strings.Contains(w.Body.String(), "BA_PASS") {
			t.Errorf("the refusal should name the setting: %q", w.Body.String())
		}
	}
	if len(k.confirms) != 0 {
		t.Error("a confirm reached the engine on a read-only instance")
	}

	// And the UI is told, so BA mode says read-only instead of failing on submit.
	body := do(t, h, "GET", "/api/health", "", nil).Body.String()
	if !strings.Contains(body, `"writes":false`) {
		t.Errorf("health = %s, want writes:false", body)
	}
	if body := do(t, h, "GET", "/api/tickets", "", nil).Code; body != 200 {
		t.Error("a read-only instance still has a queue to show")
	}
}

func TestHealthAdvertisesWritesWhenConfigured(t *testing.T) {
	body := do(t, qaServer(&fakeKnow{}, baPass), "GET", "/api/health", "", nil).Body.String()
	if !strings.Contains(body, `"writes":true`) {
		t.Errorf("health = %s, want writes:true", body)
	}
}

/* ══ The loop ══════════════════════════════════════════════════════════════════ */

func TestConfirmPassesTheAnswerThroughAndReturnsTheTicket(t *testing.T) {
	k := &fakeKnow{ticket: db.Ticket{ID: 12, Question: "How long is an invoice valid?"}}
	h := qaServer(k, baPass)

	w := do(t, h, "POST", "/api/tickets/12/confirm", `{"answer":"30 days from issue."}`, withPass(baPass))
	if w.Code != 200 {
		t.Fatalf("confirm = %d %s", w.Code, w.Body.String())
	}
	if len(k.confirms) != 1 || k.confirms[0] != "30 days from issue." {
		t.Errorf("the answer reached the engine as %+v", k.confirms)
	}
	var got db.Ticket
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got.Status != db.StatusConfirmed {
		t.Errorf("the response must carry the new status, got %q — the UI renders from it", got.Status)
	}
}

func TestARejectNeedsNoBody(t *testing.T) {
	// Dismissing takes no fields; demanding `{}` from a client with nothing to say
	// is ceremony that only shows up as a 400 in production.
	h := qaServer(&fakeKnow{ticket: db.Ticket{ID: 4}}, baPass)
	if w := do(t, h, "POST", "/api/tickets/4/reject", "", withPass(baPass)); w.Code != 200 {
		t.Errorf("reject with an empty body = %d %s", w.Code, w.Body.String())
	}
}

func TestUnknownTicketActionIs404(t *testing.T) {
	h := qaServer(&fakeKnow{}, baPass)
	if w := do(t, h, "POST", "/api/tickets/4/delete", "{}", withPass(baPass)); w.Code != http.StatusNotFound {
		t.Errorf("unknown action = %d, want 404", w.Code)
	}
}

func TestABadTicketIdIs400(t *testing.T) {
	h := qaServer(&fakeKnow{}, baPass)
	if w := do(t, h, "POST", "/api/tickets/abc/confirm", "{}", withPass(baPass)); w.Code != http.StatusBadRequest {
		t.Errorf("non-numeric id = %d, want 400", w.Code)
	}
}

func TestAMissingTicketIs404AndAStaleTransitionIs409(t *testing.T) {
	gone := qaServer(&fakeKnow{err: db.ErrNoTicket}, baPass)
	if w := do(t, gone, "POST", "/api/tickets/9/confirm", `{"answer":"x"}`, withPass(baPass)); w.Code != http.StatusNotFound {
		t.Errorf("missing ticket = %d, want 404", w.Code)
	}

	// Two BAs on the same queue: the second one's view is stale, not broken.
	stale := qaServer(&fakeKnow{err: errors.New("ticket 9 is already confirmed")}, baPass)
	w := do(t, stale, "POST", "/api/tickets/9/confirm", `{"answer":"x"}`, withPass(baPass))
	if w.Code != http.StatusConflict {
		t.Errorf("already-settled ticket = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "already confirmed") {
		t.Errorf("the reason was dropped: %q", w.Body.String())
	}
}

func TestQAEndpointsAreAbsentWithoutTheLoopWired(t *testing.T) {
	// Deps.Know is optional; nothing may 500 because it was left nil.
	h := New(Deps{Answers: &fakeAnswers{}, Index: []byte("x"), Assets: fstest.MapFS{}})
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/tickets"},
		{"POST", "/api/tickets"},
		{"POST", "/api/tickets/1/confirm"},
		{"GET", "/api/history"},
	} {
		if w := do(t, h, c.method, c.path, "{}", nil); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", c.method, c.path, w.Code)
		}
	}
}

/* ══ Regenerate ════════════════════════════════════════════════════════════════ */

func TestRegenerateAsksForAFreshAnswer(t *testing.T) {
	a := &fakeAnswers{tokens: []string{"x"}}
	h := newTestServer(a)

	do(t, h, "POST", "/api/chat", `{"question":"q"}`, nil)
	do(t, h, "POST", "/api/chat", `{"question":"q","fresh":true}`, nil)

	if len(a.asked) != 2 {
		t.Fatalf("engine saw %d asks", len(a.asked))
	}
	if a.asked[0].Fresh {
		t.Error("a normal ask asked for a fresh answer, so it can never hit the cache")
	}
	if !a.asked[1].Fresh {
		t.Error("Regenerate did not ask for a fresh answer — it would return the same cached text")
	}
}

func TestChatReportsACacheHitToTheClient(t *testing.T) {
	h := newTestServer(&fakeAnswers{tokens: []string{"cheap"}, cached: true})
	body := do(t, h, "POST", "/api/chat", `{"question":"q"}`, nil).Body.String()
	if !strings.Contains(body, `"cached":true`) {
		t.Errorf("a free answer looked identical to a paid one:\n%s", body)
	}
}
