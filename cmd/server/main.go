package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

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

	// The frontend is rendered once, for whichever asset base is configured.
	index, err := web.Index(cfg.AssetBase)
	if err != nil {
		log.Fatalf("frontend: %v", err)
	}
	if cfg.AssetBase == "/vendor" && !web.HasVendor() {
		log.Printf("warning: ASSET_BASE=/vendor but no assets are embedded — run `make vendor`, then rebuild")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /api/chat", chatHandler(engine))

	// Serve the embedded Vue single-file frontend. "/{$}" is an exact match, so
	// the FileServer below only ever sees asset paths (/vendor/...).
	indexETag := fmt.Sprintf(`"%x"`, sha256.Sum256(index))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache") // revalidate: it pins the asset versions
		w.Header().Set("ETag", indexETag)           // ...but a reload is a 304, not a re-download
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
	mux.Handle("GET /vendor/", immutable(http.FileServer(http.FS(web.FS))))

	log.Printf("Knowledge Engine on http://localhost:%s (assets: %s)", cfg.Port, cfg.AssetBase)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}

// immutable marks vendored assets as cacheable forever. Safe because every path
// carries its package version (vendor/vue@3.5.40/...), so an upgrade is a new URL
// — the same contract that makes a pinned CDN URL immutable.
func immutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
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
