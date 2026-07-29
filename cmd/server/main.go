// Command server runs mega-docs: a RAG chat API plus the embedded UI.
//
// This file is wiring only — config in, dependencies constructed, handler served.
// Retrieval lives in internal/rag, HTTP in internal/server, the UI in web/.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/config"
	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
	"knowledge-engine/internal/server"
	"knowledge-engine/web"
)

// main exists only to turn an error into an exit code. Everything else is run(), so
// that `defer store.Close()` actually runs: log.Fatal calls os.Exit, which skips every
// deferred call in the frame — and for a WAL database that means the last checkpoint
// never happens.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()

	store, err := db.Open(cfg.DBPath, cfg.EmbedDim)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer store.Close()

	// The built app, read out of the binary once. A missing build is a startup error,
	// not a blank page: the bundle is committed, so the only way to get here without one
	// is a hand-edited tree.
	index, err := web.Index()
	if err != nil {
		return fmt.Errorf("frontend: %w", err)
	}
	build, err := web.BuildInfo()
	if err != nil {
		return fmt.Errorf("frontend: %w", err)
	}

	engine := rag.New(store, ai.New(ai.Config{
		ChatBaseURL: cfg.BaseURL, EmbedBaseURL: cfg.EmbedURL,
		APIKey: cfg.APIKey, EmbedAPIKey: cfg.EmbedKey,
		EmbedModel: cfg.EmbedModel, ChatModel: cfg.ChatModel,
	}), rag.Options{TopK: cfg.TopK})
	auth := server.Auth{User: cfg.AuthUser, Pass: cfg.AuthPass}
	// The release is embedded, so a parse failure is a broken build rather than a runtime
	// condition — but it must not stop the server: the notes are the least important thing
	// here, and an instance that refuses to answer questions because its changelog is
	// malformed has its priorities inverted. Log it and carry on with no badge.
	release, err := web.ReleaseInfo()
	if err != nil {
		log.Printf("WARNING: %v — the version badge will show the commit only", err)
	}
	// Nil rather than the file's bytes when no tag has been cut: the route disappears, the
	// badge stays unclickable, and nobody opens a dialog listing nothing. The file itself is
	// always present — it has to be, `go:embed` fails to compile without it.
	var releaseJSON []byte
	if release.Version != "" {
		releaseJSON = web.ReleaseJSON()
	}
	handler := server.New(server.Deps{
		Answers: engine, Know: engine, Docs: engine, Index: index, Assets: web.FS,
		Release: releaseJSON,
		Auth:    auth, BAPass: server.BAPass(cfg.BAPass),
		// The Admin screen reads the config it is describing, so the inventory is a closure
		// over cfg rather than a snapshot: one source for what the knobs are, in the package
		// that defines them.
		Settings: func() any { return cfg.Inventory() }, AdminPass: server.AdminPass(cfg.AdminPass),
		Runtime: server.Runtime{
			Model: cfg.ChatModel, Window: cfg.Window,
			PriceIn: cfg.PriceIn, PriceOut: cfg.PriceOut,
			Version: revision(), Release: release.Version,
		},
	})

	addr := net.JoinHostPort(cfg.BindAddr, cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: an answer is a long-lived SSE stream, and a deadline
		// here would cut it off mid-generation.
	}

	// The UI's build is in the startup line because the bundle is committed: "which
	// front end is this binary serving" is otherwise unanswerable without unzipping it.
	// The revision answers the other half — `build` is a hash of web/ui alone, so a Go-only
	// deploy reuses it and would look like nothing shipped.
	log.Printf("mega-docs %s on http://%s (ui: vue %s · 8bit-nes %s · build %s, auth: %s, writes: %s, admin: %s)",
		describeRev(), addr, build.Vue, build.Nes, build.Sources[:8],
		describe(auth), writes(cfg), admin(cfg))
	warnIfExposed(cfg.BindAddr, auth)
	warnIfKeyless(cfg)
	return srv.ListenAndServe()
}

// warnIfKeyless says at startup what used to be discovered on the first question. An empty
// AI_API_KEY is *legal* — internal/ai sends no Authorization header when there is no key,
// which is correct for a keyless endpoint — so this is a warning and not a refusal, unlike
// `ingest`, which cannot do its one job without embeddings and says so as an error.
//
// The failure it replaces: a server that starts clean, serves the UI, and then answers the
// first question with a provider 401 that names neither the variable nor the file.
func warnIfKeyless(cfg config.Config) {
	if cfg.APIKey != "" {
		return
	}
	log.Printf("WARNING: AI_API_KEY is not set. Chat will fail on the first question unless")
	log.Printf("         AI_BASE_URL (%s) is a keyless endpoint. Set it in .env.", cfg.BaseURL)
}

// revision is the commit this binary was built from, short, with a `+` when the tree was
// dirty. Go stamps it into every binary built from a checkout, so there is no VERSION file
// to forget to bump and no `-ldflags -X` in the Makefile — which matters because the deploy
// is `git pull && make build`, and a version anybody has to remember to edit is a version
// that lies. Empty for `go install` from a module proxy, which carries no VCS stamp, and
// the UI prints nothing rather than a placeholder.
func revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty && rev != "" {
		rev += "+"
	}
	return rev
}

// describeRev is the startup line's version field: the revision, or a name for its absence,
// because a blank space between "mega-docs" and "on http://" reads as a formatting bug.
func describeRev() string {
	if rev := revision(); rev != "" {
		return rev
	}
	return "(no vcs stamp)"
}

func describe(a server.Auth) string {
	if a.Pass == "" {
		return "off"
	}
	return "basic as " + a.User
}

// writes reports whether a BA can confirm an answer into the corpus on this
// instance — the difference between a read-only mirror and the one that owns the
// knowledge base. Worth one word in the startup line, because getting it wrong is
// only discovered by a BA typing an answer that has nowhere to go.
func writes(cfg config.Config) string {
	if cfg.BAPass == "" {
		return "read-only (BA_PASS unset)"
	}
	return "BA into the knowledge base"
}

// admin says whether the read-only settings screen exists. One word in the startup line for
// the same reason `writes` earns one: an operator who set ADMIN_PASS wants to see that it was
// picked up, and one who did not should not go looking for a screen that is not there.
func admin(cfg config.Config) string {
	if cfg.AdminPass == "" {
		return "off (ADMIN_PASS unset)"
	}
	return "on"
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
