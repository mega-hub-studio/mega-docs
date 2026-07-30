package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"knowledge-engine/internal/db"
)

// Knowledge is the write side of the engine: the loop that turns a gap in the
// documents into part of them. Separate from Answerer so a test can fake one
// without the other, and so the read path stays describable in one sentence.
type Knowledge interface {
	Queue(limit int) (db.Queue, error)
	OpenTicket(question, miss string) (db.Ticket, error)
	Draft(id int64, answer string) (db.Ticket, error)
	// Confirm publishes the answer. `name` is what the BA wants the document called, inside
	// `qa/`; empty keeps the name it already has, or falls back to the id. A *different* name
	// on a published ticket is a rename, and the engine unpublishes the old one — so this seam
	// carries the name rather than a separate rename verb, which would be a second way to
	// change one fact.
	Confirm(ctx context.Context, id int64, answer, name string) (db.Ticket, error)
	// Retract is the way back out of `confirmed`: the document leaves retrieval and the
	// ticket becomes the draft it was, answer kept. On this interface rather than a fourth
	// seam because publishing and unpublishing are one capability held by one person.
	Retract(ctx context.Context, id int64) (db.Ticket, error)
	Reject(id int64, note string) (db.Ticket, error)
	// Delete drops the ticket row. The answer's text is a document row and documents are
	// removed softly, so this loses the question, never the words.
	Delete(ctx context.Context, id int64) error
	History(limit int) ([]db.Cached, error)
}

// maxTicket caps a ticket body. An answer is a paragraph or two of markdown; this
// is generous for that and far short of a document upload.
const maxTicket = 64 << 10 // 64 KiB

// BAPass and its gate live in gate.go, with the admin one — one compare, two secrets.

// tickets wires the whole loop onto one mux. Reads and filing a gap are open —
// a DEV who cannot report a gap will simply stop reporting them.
//
//	GET    /api/tickets                {"tickets":[…],"open":n,…}
//	POST   /api/tickets                {"question":"…","miss":"…"} → the ticket
//	POST   /api/tickets/{id}/{action}  draft | confirm | retract | reject → the ticket
//	                                   confirm takes {"answer":"…","name":"pricing-2026"}
//	DELETE /api/tickets/{id}           → {"id":n}
//	GET    /api/history                [{"question":"…","hits":n,…}]
func tickets(mux *http.ServeMux, k Knowledge, pass BAPass) {
	mux.HandleFunc("GET /api/tickets", func(w http.ResponseWriter, r *http.Request) {
		q, err := k.Queue(limitOf(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, q)
	})

	mux.HandleFunc("POST /api/tickets", func(w http.ResponseWriter, r *http.Request) {
		body, err := readTicket(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		t, err := k.OpenTicket(body.Question, body.Miss)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, t)
	})

	mux.HandleFunc("POST /api/tickets/{id}/{action}", pass.gate().wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := ticketID(w, r)
		if !ok {
			return
		}
		body, err := readTicket(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var t db.Ticket
		switch action := r.PathValue("action"); action {
		case "draft":
			t, err = k.Draft(id, body.Answer)
		case "confirm":
			t, err = k.Confirm(r.Context(), id, body.Answer, body.Name)
		case "retract":
			t, err = k.Retract(r.Context(), id)
		case "reject":
			t, err = k.Reject(id, body.Note)
		default:
			http.Error(w, "unknown action "+action, http.StatusNotFound)
			return
		}
		switch {
		case errors.Is(err, db.ErrNoTicket):
			http.Error(w, err.Error(), http.StatusNotFound)
		case err != nil:
			// A transition that no longer applies (retracting what was never published,
			// dismissing what is already dismissed) is the client's stale view, not a
			// server fault.
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			writeJSON(w, t)
		}
	}))

	// DELETE /api/tickets/{id} — drop the question itself.
	//
	// Separate from the {action} route rather than a fifth verb on it, because it is the one
	// move that returns no ticket: there is nothing left to render. Gated like every other
	// write, and it takes the answer's document out of retrieval on the way — a deleted
	// ticket whose answer went on being cited would be the same disagreement between the
	// queue and the corpus that made `confirmed` a trap.
	mux.HandleFunc("DELETE /api/tickets/{id}", pass.gate().wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := ticketID(w, r)
		if !ok {
			return
		}
		switch err := k.Delete(r.Context(), id); {
		case errors.Is(err, db.ErrNoTicket):
			http.Error(w, err.Error(), http.StatusNotFound)
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		default:
			writeJSON(w, deleted{ID: id})
		}
	}))

	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		h, err := k.History(limitOf(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, h)
	})
}

// deleted is all a delete has left to say. The id is echoed rather than answered with 204
// so a client can match the response to the row it removed from a list it may have
// re-fetched in between.
type deleted struct {
	ID int64 `json:"id"`
}

// ticketID parses the id both write routes share and answers the client itself when it is
// not a number, so neither handler carries a copy of that decision.
func ticketID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad ticket id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

type ticketBody struct {
	Question string `json:"question"`
	Miss     string `json:"miss"`
	Answer   string `json:"answer"`
	Note     string `json:"note"`
	// Name is what to call the document a confirm publishes, inside qa/. Optional: absent
	// keeps whatever the ticket already has, which is what stops a client that does not know
	// about this field from renaming a document by leaving it out.
	Name string `json:"name"`
}

// readTicket tolerates an empty body: reject takes no fields, and requiring `{}`
// from a client that has nothing to say is ceremony.
func readTicket(r *http.Request) (ticketBody, error) {
	var b ticketBody
	err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxTicket)).Decode(&b)
	if err != nil && !errors.Is(err, io.EOF) {
		return b, errBadRequest
	}
	b.Question = strings.TrimSpace(b.Question)
	b.Answer = strings.TrimSpace(b.Answer)
	b.Name = strings.TrimSpace(b.Name)
	return b, nil
}

func limitOf(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n // <=0 lets the store pick its default
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
