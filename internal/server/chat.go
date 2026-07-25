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
//	done      {"done":true}
//	error     {"message":"…"}                  instead of citations/done
//
// The error arrives *in the stream* because the status line is already sent by the
// time generation can fail — the client shows it on the turn either way.
func chatHandler(answers Answerer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		question, err := readQuestion(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s, err := newStream(w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cites, err := answers.Answer(r.Context(), question, func(tok string) {
			s.send("token", map[string]string{"t": tok})
		})
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
		if cites == nil {
			cites = []rag.Citation{}
		}
		s.send("citations", cites)
		s.send("done", map[string]bool{"done": true})
	}
}

func readQuestion(r *http.Request) (string, error) {
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxQuestion)).Decode(&body); err != nil {
		return "", errBadRequest
	}
	q := strings.TrimSpace(body.Question)
	if q == "" {
		return "", errBadRequest
	}
	return q, nil
}
