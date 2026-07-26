// Command server runs the Knowledge Engine: a RAG chat API plus the embedded UI.
//
// This file is wiring only — config in, dependencies constructed, handler served.
// Retrieval lives in internal/rag, HTTP in internal/server, the UI in web/.
package main

import (
	"log"
	"net/http"
	"time"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/config"
	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
	"knowledge-engine/internal/server"
	"knowledge-engine/web"
)

func main() {
	cfg := config.Load()

	store, err := db.Open(cfg.DBPath, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	// The page is rendered once, for whichever asset base is configured.
	index, err := web.Index(cfg.AssetBase)
	if err != nil {
		log.Fatalf("frontend: %v", err)
	}
	if cfg.AssetBase == config.VendorAssetBase && !web.HasVendor() {
		log.Printf("warning: ASSET_BASE=%s but no assets are embedded — run `make vendor`, then rebuild",
			config.VendorAssetBase)
	}

	engine := rag.New(store, ai.New(cfg.BaseURL, cfg.EmbedURL, cfg.APIKey, cfg.EmbedModel, cfg.ChatModel), cfg.TopK)
	handler := server.New(server.Deps{Answers: engine, Index: index, Assets: web.FS})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: an answer is a long-lived SSE stream, and a deadline
		// here would cut it off mid-generation.
	}

	log.Printf("Knowledge Engine on http://localhost:%s (assets: %s)", cfg.Port, cfg.AssetBase)
	log.Fatal(srv.ListenAndServe())
}
