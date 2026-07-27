// Package server is the HTTP layer: routing, cache policy, and the SSE chat
// endpoint. It knows nothing about SQLite, embeddings or templates — it is handed
// narrow interfaces and the already-rendered page, so it can be tested with neither.
// The read side (Answerer) is required; the write sides (Knowledge, Importer) are
// nil-able, and their routes disappear rather than half-work.
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

// Answerer is the read side of the RAG engine — all the HTTP layer needs to answer
// a question. *rag.Engine satisfies it; tests pass a fake.
type Answerer interface {
	Answer(ctx context.Context, a rag.Ask) (rag.Reply, error)
	// Corpus answers "what does this engine know?" — without it, an empty index
	// is indistinguishable from a broken retriever.
	Corpus(limit int) (db.Corpus, error)
}

// Deps is everything the handler set needs.
type Deps struct {
	Answers Answerer  // the RAG engine, read side
	Know    Knowledge // the QA loop; nil leaves those routes unregistered
	Docs    Importer  // document import; nil leaves that route unregistered
	Index   []byte    // index.html, already rendered for the configured asset base
	Assets  fs.FS     // embedded static tree: app/… and vendor/…
	Auth    Auth      // optional Basic credentials; zero value = open
	BAPass  BAPass    // gates the write actions; empty = no write surface
	Runtime Runtime   // what the status line reports; zero values simply hide fields
}

// Runtime is what an answer costs and what produced it — everything the status line
// shows that the engine cannot infer per request. Zero means "unknown", and the UI
// prints nothing rather than a zero, because a cost of $0.00 and an unmeasured cost
// are different facts.
type Runtime struct {
	Model    string
	Window   int
	PriceIn  float64
	PriceOut float64
}

// New wires the routes and returns the whole app as one handler.
//
// This is the app and nothing else. The guide is documentation with its own public
// domain, so it is not served here — one surface, one job.
//
//	GET  /            index.html          revalidated (it pins asset versions)
//	GET  /api/health  {"ok":true,"writes":bool} — open, so probes need no secret
//	POST /api/chat    SSE: cached · token · citations · done · error
//	GET  /api/corpus  {"docs":n,"chunks":n,"approved":n,"documents":[…]}
//	GET  /api/tickets · POST /api/tickets · POST /api/tickets/{id}/{action}
//	POST /api/documents  import .md/.txt into the corpus — same gate as a confirm
//	GET  /api/history answers still free to replay
//	GET  /app/…       app modules + CSS   revalidated, ETag'd from the binary
//	GET  /vendor/…    vendored CDN assets immutable (version is in the path)
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	// What the status line needs before the first question, plus the one field BA
	// mode needs: whether it can do anything here. That this instance *has* a
	// password configured is not a secret — that it is read-only is exactly what a
	// BA needs to be told before typing an answer.
	//
	// The model name and prices are deliberate disclosure, not a leak: an operator
	// asked for them on screen. The engine itself still refuses to discuss them —
	// that rule is about what a *document* answer may contain.
	health := fmt.Sprintf(`{"ok":true,"writes":%t,"model":%q,"window":%d,"price_in":%g,"price_out":%g}`,
		d.BAPass.enabled(), d.Runtime.Model, d.Runtime.Window, d.Runtime.PriceIn, d.Runtime.PriceOut)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The body is a constant; a failed write means the probe hung up, which is its
		// business, not ours.
		_, _ = w.Write([]byte(health))
	})

	mux.HandleFunc("POST /api/chat", chatHandler(d.Answers))
	mux.HandleFunc("GET /api/corpus", corpusHandler(d.Answers))
	if d.Know != nil {
		tickets(mux, d.Know, d.BAPass)
	}
	if d.Docs != nil {
		documents(mux, d.Docs, d.BAPass)
	}

	// "/{$}" matches only the root, so the file servers below never see it.
	mux.Handle("GET /{$}", revalidate(etag(d.Index), serveBytes(d.Index, "text/html; charset=utf-8")))
	// The app tree changes only when the binary does, so one ETag over the whole
	// tree is enough to invalidate it — and costs one 304 instead of a re-download.
	files := http.FileServerFS(d.Assets)
	mux.Handle("GET /app/", revalidate(etagFS(d.Assets, "app"), files))
	mux.Handle("GET /vendor/", immutable(files))

	return guard(d.Auth, mux)
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
	// The tree is an embed.FS: a read cannot fail at runtime, and if it somehow did the
	// tag would simply change, which is the safe direction for a cache key.
	_ = fs.WalkDir(fsys, root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s:%x\n", path, sha256.Sum256(b)) // writing to a hash cannot fail
		return nil
	})
	return fmt.Sprintf(`"%x"`, h.Sum(nil)[:16])
}

// match handles the one If-None-Match form browsers actually send back, plus "*".
func match(header, tag string) bool {
	return header != "" && (header == "*" || header == tag || header == "W/"+tag)
}
