// Package server is the HTTP layer: routing, cache policy, and the SSE chat
// endpoint. It knows nothing about SQLite, embeddings or templates — it is handed
// an Answerer and the already-rendered page, so it can be tested with neither.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
)

// Answerer is the one thing the HTTP layer needs from the RAG engine.
// *rag.Engine satisfies it; tests pass a fake.
type Answerer interface {
	Answer(ctx context.Context, question string, onToken func(string)) ([]rag.Citation, error)
	// Corpus answers "what does this engine know?" — without it, an empty index
	// is indistinguishable from a broken retriever.
	Corpus(limit int) (db.Corpus, error)
}

// Deps is everything the handler set needs. All fields are required.
type Deps struct {
	Answers Answerer // the RAG engine
	Index   []byte   // index.html, already rendered for the configured asset base
	Assets  fs.FS    // embedded static tree: app/… and vendor/…
}

// New wires the routes and returns the whole app as one handler.
//
//	GET  /            index.html          revalidated (it pins asset versions)
//	GET  /api/health  {"ok":true}
//	POST /api/chat    SSE: token · citations · done · error
//	GET  /api/corpus  {"docs":n,"chunks":n,"approved":n,"documents":[…]}
//	GET  /app/…       app modules + CSS   revalidated, ETag'd from the binary
//	GET  /vendor/…    vendored CDN assets immutable (version is in the path)
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("POST /api/chat", chatHandler(d.Answers))
	mux.HandleFunc("GET /api/corpus", corpusHandler(d.Answers))

	// "/{$}" matches only the root, so the file servers below never see it.
	mux.Handle("GET /{$}", revalidate(etag(d.Index), serveBytes(d.Index, "text/html; charset=utf-8")))

	// The app tree changes only when the binary does, so one ETag over the whole
	// tree is enough to invalidate it — and costs one 304 instead of a re-download.
	files := http.FileServerFS(d.Assets)
	mux.Handle("GET /app/", revalidate(etagFS(d.Assets, "app"), files))
	mux.Handle("GET /vendor/", immutable(files))

	return mux
}

func serveBytes(b []byte, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
	})
}

// revalidate lets the browser cache a response but check before reusing it. With
// a strong ETag that check is a 304 — no bytes, and never a stale asset after a
// deploy.
func revalidate(tag string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "no-cache")
		if match(r.Header.Get("If-None-Match"), tag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// immutable is safe only where the URL carries a version (vendor/vue@3.5.40/…),
// which is exactly the contract that makes a pinned CDN URL cacheable for a year.
func immutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

func etag(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf(`"%x"`, sum[:16])
}

// etagFS hashes every file under root, so any edit anywhere changes the tag.
func etagFS(fsys fs.FS, root string) string {
	h := sha256.New()
	fs.WalkDir(fsys, root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s:%x\n", path, sha256.Sum256(b))
		return nil
	})
	return fmt.Sprintf(`"%x"`, h.Sum(nil)[:16])
}

// match handles the one If-None-Match form browsers actually send back, plus "*".
func match(header, tag string) bool {
	return header != "" && (header == "*" || header == tag || header == "W/"+tag)
}
