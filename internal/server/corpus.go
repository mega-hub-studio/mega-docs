package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// defaultCorpusLimit keeps the payload phone-sized; ?limit= raises it.
const defaultCorpusLimit = 100

// corpusHandler reports what is indexed. The UI shows it on the empty state, so
// "nothing is ingested yet" reads as a state, not as a wrong answer.
func corpusHandler(answers Answerer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := defaultCorpusLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}

		corpus, err := answers.Corpus(limit)
		if err != nil {
			log.Printf("corpus: %v", err)
			http.Error(w, "corpus unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store") // it changes on every ingest
		if err := json.NewEncoder(w).Encode(corpus); err != nil {
			log.Printf("corpus encode: %v", err)
		}
	}
}
