package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"knowledge-engine/internal/rag"
)

// maxAsk caps the request body. A question is a sentence, but it arrives with the tail of
// the conversation attached — the server keeps no session, so the thread rides along with
// every follow-up. The cap is therefore the size of a few exchanges rather than of one
// sentence; the client sends the last few turns and this is what stops a client from
// deciding to send a thousand.
const maxAsk = 64 << 10 // 64 KiB

var errBadRequest = errors.New("bad request")

// chatHandler answers one question over SSE:
//
//	token     {"t":"…"}                       repeated, as the model streams
//	citations [{"n":1,"doc":"…","heading":"…"}] once, after the answer
//	done      {"done":true,"cached":bool}
//	error     {"message":"…"}                  instead of citations/done
//
// The error arrives *in the stream* because the status line is already sent by the
// time generation can fail — the client shows it on the turn either way.
func chatHandler(answers Answerer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ask, err := readQuestion(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s, err := newStream(w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ask.OnToken = func(tok string) { s.send("token", map[string]string{"t": tok}) }
		reply, err := answers.Answer(r.Context(), ask)
		if err != nil {
			// A client that navigated away cancels the context; that's not worth
			// reporting to a connection which is already gone.
			if r.Context().Err() == nil {
				s.send("error", map[string]string{"message": err.Error()})
			}
			return
		}
		// Never emit `null` here: the engine returns nil citations when retrieval
		// found nothing, and the client reads .length off this straight away.
		if reply.Citations == nil {
			reply.Citations = []rag.Citation{}
		}
		s.send("citations", reply.Citations)
		// cached and the token counts ride on `done` rather than their own events:
		// both are known only once the answer is complete, and one frame keeps the
		// client's switch small. A struct, not a map, so the field order is stable.
		//
		// The counts are omitted when the provider reported none, so the status line
		// can distinguish "free, it was cached" from "spent, but unmeasured" — a
		// zero printed as a cost would be a lie in the second case.
		done := struct {
			Done   bool `json:"done"`
			Cached bool `json:"cached"`
			In     int  `json:"in,omitempty"`
			Out    int  `json:"out,omitempty"`
		}{Done: true, Cached: reply.Cached}
		if reply.Usage.Reported() {
			done.In, done.Out = reply.Usage.PromptTokens, reply.Usage.CompletionTokens
		}
		s.send("done", done)
	}
}

// readQuestion parses the request into the engine's own Ask, minus the callback the
// handler owns. `fresh` is Regenerate: the one case where a cached answer is the
// wrong answer, because the user just told us it was.
func readQuestion(r *http.Request) (rag.Ask, error) {
	var body struct {
		Question string `json:"question"`
		Scope    string `json:"scope"` // a document or folder to answer from; "" = all
		Fresh    bool   `json:"fresh"`
		// History is the thread this question continues, oldest first — decoded straight
		// into the engine's own type, because a second spelling of a turn would be a
		// second thing to keep in step with the wire.
		History []rag.Turn `json:"history"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxAsk)).Decode(&body); err != nil {
		return rag.Ask{}, errBadRequest
	}
	q := strings.TrimSpace(body.Question)
	if q == "" {
		return rag.Ask{}, errBadRequest
	}
	// The engine canonicalises the scope rather than the handler: it is part of the
	// cache key, so exactly one place may decide what "booking/" means. The history is
	// passed through untouched for the same reason: dropping an unanswered turn is a
	// decision about what a model may read, and that belongs with the prompt.
	return rag.Ask{Question: q, Scope: body.Scope, Fresh: body.Fresh, History: body.History}, nil
}
