package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"knowledge-engine/internal/rag"
)

// maxQuestion caps the request body. A question is a sentence, not an upload.
const maxQuestion = 8 << 10 // 8 KiB

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
		// cached rides on `done` rather than its own event: it is known only once
		// the answer is complete, and one frame keeps the client's switch small.
		// A struct, not a map, so the frame's field order is stable.
		s.send("done", struct {
			Done   bool `json:"done"`
			Cached bool `json:"cached"`
		}{true, reply.Cached})
	}
}

// readQuestion parses the request into the engine's own Ask, minus the callback the
// handler owns. `fresh` is Regenerate: the one case where a cached answer is the
// wrong answer, because the user just told us it was.
func readQuestion(r *http.Request) (rag.Ask, error) {
	var body struct {
		Question string `json:"question"`
		Fresh    bool   `json:"fresh"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxQuestion)).Decode(&body); err != nil {
		return rag.Ask{}, errBadRequest
	}
	q := strings.TrimSpace(body.Question)
	if q == "" {
		return rag.Ask{}, errBadRequest
	}
	return rag.Ask{Question: q, Fresh: body.Fresh}, nil
}
