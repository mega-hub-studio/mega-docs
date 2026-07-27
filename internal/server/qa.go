package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
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
	Confirm(ctx context.Context, id int64, answer string) (db.Ticket, error)
	Reject(id int64, note string) (db.Ticket, error)
	History(limit int) ([]db.Cached, error)
}

// maxTicket caps a ticket body. An answer is a paragraph or two of markdown; this
// is generous for that and far short of a document upload.
const maxTicket = 64 << 10 // 64 KiB

// BAPass gates every action that changes what the engine will say: confirming an
// answer into the corpus, dismissing a question, and importing a document.
//
// Reads stay open. This app has no accounts, so the password is the whole
// difference between "anyone on the tailnet can read the documents" — which is the
// point of it — and "anyone on the tailnet can rewrite them", which is not.
//
// An unset password means no write surface at all, not open writes: forgetting to
// configure a secret must never be the way you end up without one.
type BAPass string

func (p BAPass) enabled() bool { return p != "" }

// gate wraps the handlers that write. 403 when writes are off (nothing to unlock),
// 401 when the password is wrong (retry with a different one).
func (p BAPass) gate(h http.HandlerFunc) http.HandlerFunc {
	if !p.enabled() {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "writes are disabled: BA_PASS is not set on this instance", http.StatusForbidden)
		}
	}
	want := sha256.Sum256([]byte(p))
	return func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get("X-BA-Pass")))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			http.Error(w, "wrong BA password", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// tickets wires the whole loop onto one mux. Reads and filing a gap are open —
// a DEV who cannot report a gap will simply stop reporting them.
//
//	GET  /api/tickets                {"tickets":[…],"open":n,…}
//	POST /api/tickets                {"question":"…","miss":"…"} → the ticket
//	POST /api/tickets/{id}/{action}  draft | confirm | reject    → the ticket
//	GET  /api/history                [{"question":"…","hits":n,…}]
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

	mux.HandleFunc("POST /api/tickets/{id}/{action}", pass.gate(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad ticket id", http.StatusBadRequest)
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
			t, err = k.Confirm(r.Context(), id, body.Answer)
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
			// A transition that no longer applies (already confirmed, already
			// dismissed) is the client's stale view, not a server fault.
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			writeJSON(w, t)
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

type ticketBody struct {
	Question string `json:"question"`
	Miss     string `json:"miss"`
	Answer   string `json:"answer"`
	Note     string `json:"note"`
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
