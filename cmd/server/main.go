// Command server runs the Knowledge Engine: a RAG chat API plus the embedded UI.
//
// This file is wiring only — config in, dependencies constructed, handler served.
// Retrieval lives in internal/rag, HTTP in internal/server, the UI in web/.
package main

import (
	"log"
	"net"
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
	index, err := web.Index(cfg.AssetBase, cfg.SiteURL)
	if err != nil {
		log.Fatalf("frontend: %v", err)
	}
	if cfg.AssetBase == config.VendorAssetBase && !web.HasVendor() {
		log.Printf("warning: ASSET_BASE=%s but no assets are embedded — run `make vendor`, then rebuild",
			config.VendorAssetBase)
	}

	engine := rag.New(store, ai.New(ai.Config{
		ChatBaseURL: cfg.BaseURL, EmbedBaseURL: cfg.EmbedURL,
		APIKey: cfg.APIKey, EmbedAPIKey: cfg.EmbedKey,
		EmbedModel: cfg.EmbedModel, ChatModel: cfg.ChatModel,
	}), cfg.TopK)
	auth := server.Auth{User: cfg.AuthUser, Pass: cfg.AuthPass}
	handler := server.New(server.Deps{
		Answers: engine, Index: index, Assets: web.FS, Auth: auth,
	})

	addr := net.JoinHostPort(cfg.BindAddr, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: an answer is a long-lived SSE stream, and a deadline
		// here would cut it off mid-generation.
	}

	log.Printf("Knowledge Engine on http://%s (assets: %s, auth: %s)", addr, cfg.AssetBase, describe(auth))
	warnIfExposed(cfg.BindAddr, auth)
	log.Fatal(srv.ListenAndServe())
}

func describe(a server.Auth) string {
	if a.Pass == "" {
		return "off"
	}
	return "basic as " + a.User
}

// warnIfExposed says the quiet part out loud: this app has no access control of
// its own, so reaching past loopback without AUTH_PASS means whoever can route to
// the port can read every indexed document and spend the provider key.
func warnIfExposed(bind string, a server.Auth) {
	if a.Enabled() || isLoopback(bind) {
		return
	}
	log.Printf("WARNING: bound to %s with no auth — anyone who can reach this port can read every", bind)
	log.Printf("         indexed document. Either set AUTH_PASS, or keep it inside a tailnet/VPN.")
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
