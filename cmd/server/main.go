package main

import (
	"encoding/json"
	"log"
	"net/http"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/config"
	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
	"knowledge-engine/web"
)

func main() {
	cfg := config.Load()

	store, err := db.Open(cfg.DBPath, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	client := ai.New(cfg.BaseURL, cfg.APIKey, cfg.EmbedModel, cfg.ChatModel)
	engine := rag.New(store, client, cfg.TopK)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /api/chat", chatHandler(engine))

	// Serve embedded Vue single-file frontend.
	mux.Handle("/", http.FileServer(http.FS(web.FS)))

	log.Printf("Knowledge Engine on http://localhost:%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}

func chatHandler(engine *rag.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Question == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		send := func(event string, v any) {
			b, _ := json.Marshal(v)
			w.Write([]byte("event: " + event + "\ndata: "))
			w.Write(b)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}

		cites, err := engine.Answer(r.Context(), body.Question, func(tok string) {
			send("token", map[string]string{"t": tok})
		})
		if err != nil {
			send("error", map[string]string{"message": err.Error()})
			return
		}
		send("citations", cites)
		send("done", map[string]bool{"done": true})
	}
}
